package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/rs/zerolog/log"
)

type AiderHandler struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	stderr       io.ReadCloser
	mu           sync.Mutex
	initialized  bool
	workDir      string
}

func NewAiderHandler(workDir string) *AiderHandler {
	return &AiderHandler{
		workDir: workDir,
	}
}

func (a *AiderHandler) Initialize(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.initialized {
		return nil
	}

	// Try different Aider implementations in order of preference
	if _, err := exec.LookPath("aider"); err == nil {
		// Try real aider first
		log.Info().Msg("attempting to use real aider CLI")
		a.cmd = exec.CommandContext(ctx, "aider", "--no-git", "--yes-always", "--no-pretty")
	} else if _, err := os.Stat("./aider-wrapper.py"); err == nil {
		// Use our Python wrapper with API integration
		log.Info().Msg("using aider-wrapper.py with API integration")
		a.cmd = exec.CommandContext(ctx, "./aider-wrapper.py")
	} else {
		// Fallback to simple mock
		log.Info().Msg("using echo-aider.py mock")
		a.cmd = exec.CommandContext(ctx, "./echo-aider.py")
	}
	a.cmd.Dir = a.workDir

	var err error
	a.stdin, err = a.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	a.stdout, err = a.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	a.stderr, err = a.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("start aider: %w", err)
	}

	go a.logStderr()

	a.initialized = true
	log.Info().Str("workDir", a.workDir).Msg("aider initialized")

	return nil
}

func (a *AiderHandler) HandleChatMessage(ctx context.Context, msg *protocol.ChatMessage) (<-chan *protocol.ChatReply, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	replies := make(chan *protocol.ChatReply, 10)

	go func() {
		defer close(replies)

		a.mu.Lock()
		_, err := fmt.Fprintf(a.stdin, "%s\n", msg.Content)
		a.mu.Unlock()

		if err != nil {
			log.Error().Err(err).Msg("write to aider failed")
			return
		}

		scanner := bufio.NewScanner(a.stdout)
		scanner.Split(scanStreamTokens)

		timeout := time.NewTimer(60 * time.Second)
		defer timeout.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timeout.C:
				replies <- &protocol.ChatReply{
					Content:  "\n[Response timeout]",
					Finished: true,
				}
				return
			default:
				if scanner.Scan() {
					token := scanner.Text()
					
					timeout.Reset(60 * time.Second)
					
					isPrompt := strings.HasSuffix(token, "> ") || 
					           strings.HasSuffix(token, "? ") ||
					           strings.Contains(token, "aider>")
					
					replies <- &protocol.ChatReply{
						Content:  token,
						Finished: isPrompt,
					}
					
					if isPrompt {
						return
					}
				} else {
					if err := scanner.Err(); err != nil {
						log.Error().Err(err).Msg("scanner error")
					}
					replies <- &protocol.ChatReply{
						Content:  "",
						Finished: true,
					}
					return
				}
			}
		}
	}()

	return replies, nil
}

func (a *AiderHandler) logStderr() {
	scanner := bufio.NewScanner(a.stderr)
	for scanner.Scan() {
		log.Debug().Str("source", "aider").Msg(scanner.Text())
	}
}

func (a *AiderHandler) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.initialized {
		return nil
	}

	if a.stdin != nil {
		a.stdin.Close()
	}

	if a.cmd != nil && a.cmd.Process != nil {
		a.cmd.Process.Kill()
		a.cmd.Wait()
	}

	a.initialized = false
	return nil
}

func scanStreamTokens(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			return i + 1, data[:i+1], nil
		}
		
		if i > 0 && (data[i] == ' ' || data[i] == '.' || data[i] == ',' || data[i] == '!' || data[i] == '?') {
			return i + 1, data[:i+1], nil
		}
	}

	if len(data) > 50 {
		for i := 50; i > 0; i-- {
			if data[i] == ' ' {
				return i + 1, data[:i+1], nil
			}
		}
		return 50, data[:50], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}