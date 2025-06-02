package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// FileWatcher monitors file system changes in the work directory
type FileWatcher struct {
	workDir     string
	watcher     *fsnotify.Watcher
	context     *ConversationContext
	mu          sync.RWMutex
	watchedDirs map[string]bool
	debouncer   *EventDebouncer
	
	// Channels for communication
	eventChan   chan FileEvent
	ctx         context.Context
	cancel      context.CancelFunc
}

// FileEvent represents a file system event
type FileEvent struct {
	Path      string            `json:"path"`
	Operation string            `json:"operation"` // create, write, remove, rename
	Timestamp time.Time         `json:"timestamp"`
	Size      int64             `json:"size,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// EventDebouncer prevents rapid-fire events for the same file
type EventDebouncer struct {
	events map[string]*time.Timer
	delay  time.Duration
	mu     sync.Mutex
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(workDir string, context *ConversationContext) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		workDir:     workDir,
		watcher:     watcher,
		context:     context,
		watchedDirs: make(map[string]bool),
		debouncer:   NewEventDebouncer(500 * time.Millisecond),
		eventChan:   make(chan FileEvent, 100),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start watching
	go fw.watchLoop()

	// Add initial directories to watch
	if err := fw.addInitialWatches(); err != nil {
		fw.Close()
		return nil, fmt.Errorf("failed to add initial watches: %w", err)
	}

	log.Info().
		Str("workDir", workDir).
		Msg("file watcher initialized")

	return fw, nil
}

// NewEventDebouncer creates a new event debouncer
func NewEventDebouncer(delay time.Duration) *EventDebouncer {
	return &EventDebouncer{
		events: make(map[string]*time.Timer),
		delay:  delay,
	}
}

// addInitialWatches adds the work directory and important subdirectories to the watch list
func (fw *FileWatcher) addInitialWatches() error {
	// Always watch the root work directory
	if err := fw.addWatch(fw.workDir); err != nil {
		return err
	}

	// Watch common development directories
	commonDirs := []string{
		"src", "lib", "app", "components", "utils", "services",
		"internal", "pkg", "cmd", "api", "handlers", "models",
		"tests", "test", "__tests__", "spec",
		"docs", "scripts", "config",
	}

	for _, dir := range commonDirs {
		dirPath := filepath.Join(fw.workDir, dir)
		if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
			if err := fw.addWatch(dirPath); err != nil {
				log.Error().Err(err).Str("dir", dirPath).Msg("failed to watch directory")
			}
		}
	}

	// Watch files already in the conversation context
	for filePath := range fw.context.Files {
		dir := filepath.Dir(filepath.Join(fw.workDir, filePath))
		if err := fw.addWatch(dir); err != nil {
			log.Error().Err(err).Str("dir", dir).Msg("failed to watch context file directory")
		}
	}

	return nil
}

// addWatch adds a directory to the watch list
func (fw *FileWatcher) addWatch(path string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Check if already watching
	if fw.watchedDirs[path] {
		return nil
	}

	// Add to fsnotify watcher
	if err := fw.watcher.Add(path); err != nil {
		return fmt.Errorf("failed to add watch for %s: %w", path, err)
	}

	fw.watchedDirs[path] = true
	
	log.Debug().
		Str("path", path).
		Msg("added directory to file watcher")

	return nil
}

// WatchFile adds a specific file's directory to the watch list
func (fw *FileWatcher) WatchFile(filePath string) error {
	absPath := filepath.Join(fw.workDir, filePath)
	dir := filepath.Dir(absPath)
	return fw.addWatch(dir)
}

// watchLoop is the main event processing loop
func (fw *FileWatcher) watchLoop() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleFsEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("file watcher error")

		case <-fw.ctx.Done():
			return
		}
	}
}

// handleFsEvent processes a file system event
func (fw *FileWatcher) handleFsEvent(event fsnotify.Event) {
	// Filter out irrelevant files
	if fw.shouldIgnoreFile(event.Name) {
		return
	}

	// Convert to relative path
	relPath, err := filepath.Rel(fw.workDir, event.Name)
	if err != nil {
		log.Error().Err(err).Str("path", event.Name).Msg("failed to get relative path")
		return
	}

	// Create file event
	fileEvent := FileEvent{
		Path:      relPath,
		Operation: fw.fsEventToOperation(event.Op),
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	// Add file size for write operations
	if event.Op&fsnotify.Write == fsnotify.Write {
		if stat, err := os.Stat(event.Name); err == nil {
			fileEvent.Size = stat.Size()
		}
	}

	// Debounce the event
	fw.debouncer.Debounce(relPath, func() {
		fw.processFileEvent(fileEvent)
	})
}

// shouldIgnoreFile determines if a file should be ignored
func (fw *FileWatcher) shouldIgnoreFile(path string) bool {
	name := filepath.Base(path)
	
	// Ignore hidden files and directories
	if strings.HasPrefix(name, ".") && name != ".env" && name != ".gitignore" {
		return true
	}

	// Ignore common build/cache directories
	ignoreDirs := []string{
		"node_modules", ".git", ".svn", ".hg",
		"build", "dist", "target", "bin", "obj",
		".next", ".nuxt", ".cache", "coverage",
		"__pycache__", ".pytest_cache", ".mypy_cache",
		"vendor", ".vendor", "Godeps",
	}

	for _, ignoreDir := range ignoreDirs {
		if strings.Contains(path, string(filepath.Separator)+ignoreDir+string(filepath.Separator)) ||
			strings.HasSuffix(path, string(filepath.Separator)+ignoreDir) {
			return true
		}
	}

	// Ignore temporary and backup files
	if strings.HasSuffix(name, "~") ||
		strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, ".temp") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".swo") ||
		strings.HasPrefix(name, "#") {
		return true
	}

	// Ignore log files
	if strings.HasSuffix(name, ".log") {
		return true
	}

	return false
}

// fsEventToOperation converts fsnotify events to our operation strings
func (fw *FileWatcher) fsEventToOperation(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return "create"
	case op&fsnotify.Write == fsnotify.Write:
		return "write"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "remove"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "rename"
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return "chmod"
	default:
		return "unknown"
	}
}

// processFileEvent processes a debounced file event
func (fw *FileWatcher) processFileEvent(event FileEvent) {
	log.Debug().
		Str("path", event.Path).
		Str("operation", event.Operation).
		Int64("size", event.Size).
		Msg("processing file event")

	// Update conversation context
	var role string
	switch event.Operation {
	case "create":
		role = "created"
		// Auto-watch the directory of new files
		if dir := filepath.Dir(filepath.Join(fw.workDir, event.Path)); dir != "." {
			fw.addWatch(dir)
		}
	case "write":
		role = "active"
	case "remove":
		role = "deleted"
	default:
		role = "modified"
	}

	if err := fw.context.UpdateFileContext(event.Path, role); err != nil {
		log.Error().Err(err).
			Str("path", event.Path).
			Str("operation", event.Operation).
			Msg("failed to update file context")
	}

	// Send event to channel for external processing
	select {
	case fw.eventChan <- event:
	default:
		log.Warn().Msg("file event channel full, dropping event")
	}
}

// Debounce adds or updates a debounced function call
func (ed *EventDebouncer) Debounce(key string, fn func()) {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	// Cancel existing timer
	if timer, exists := ed.events[key]; exists {
		timer.Stop()
	}

	// Create new timer
	ed.events[key] = time.AfterFunc(ed.delay, func() {
		fn()
		ed.mu.Lock()
		delete(ed.events, key)
		ed.mu.Unlock()
	})
}

// Events returns the file event channel
func (fw *FileWatcher) Events() <-chan FileEvent {
	return fw.eventChan
}

// GetWatchedDirectories returns a list of currently watched directories
func (fw *FileWatcher) GetWatchedDirectories() []string {
	fw.mu.RLock()
	defer fw.mu.RUnlock()

	dirs := make([]string, 0, len(fw.watchedDirs))
	for dir := range fw.watchedDirs {
		dirs = append(dirs, dir)
	}
	return dirs
}

// Close stops the file watcher and cleans up resources
func (fw *FileWatcher) Close() error {
	fw.cancel()
	
	if fw.watcher != nil {
		if err := fw.watcher.Close(); err != nil {
			return fmt.Errorf("failed to close fsnotify watcher: %w", err)
		}
	}

	close(fw.eventChan)

	log.Info().Msg("file watcher closed")
	return nil
}