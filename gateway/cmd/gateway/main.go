package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devtail/gateway/internal/chat"
	"github.com/devtail/gateway/internal/terminal"
	ws "github.com/devtail/gateway/internal/websocket"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	port     string
	workDir  string
	logLevel string
	useMock  bool
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gateway",
		Short: "DevTail Gateway - WebSocket multiplexer for AI development",
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&port, "port", "p", "8080", "Port to listen on")
	rootCmd.Flags().StringVarP(&workDir, "workdir", "w", ".", "Working directory for Aider")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().BoolVar(&useMock, "mock", false, "Use mock Aider implementation")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("failed to execute command")
	}
}

func run(cmd *cobra.Command, args []string) {
	setupLogging()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	chatHandler := chat.NewHandler(workDir, useMock)
	defer chatHandler.Close()

	// Create terminal manager
	terminalManager := terminal.NewManager(
		terminal.WithMaxSessions(20),
		terminal.WithSessionTimeout(30*time.Minute),
		terminal.WithDefaultShell("/bin/bash"),
	)
	defer terminalManager.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWebSocket(chatHandler, terminalManager))
	mux.HandleFunc("/health", handleHealth)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("port", port).Msg("starting gateway server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-sigCh
	log.Info().Msg("shutting down server")

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown failed")
	}
}

func handleWebSocket(chatHandler chat.Handler, terminalManager *terminal.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Msg("websocket upgrade failed")
			return
		}

		handler := ws.NewUnifiedHandler(conn, chatHandler, terminalManager)
		
		log.Info().
			Str("remote", r.RemoteAddr).
			Str("user-agent", r.UserAgent()).
			Msg("new websocket connection")

		handler.Run()

		log.Info().
			Str("remote", r.RemoteAddr).
			Msg("websocket connection closed")
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"gateway"}`))
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	if os.Getenv("GATEWAY_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}