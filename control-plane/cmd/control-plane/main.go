package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devtail/control-plane/api"
	"github.com/devtail/control-plane/internal/hetzner"
	"github.com/devtail/control-plane/internal/tailscale"
	"github.com/devtail/control-plane/internal/vm"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "control-plane",
		Short: "DevTail Control Plane - VM provisioning and management",
		Run:   run,
	}

	rootCmd.PersistentFlags().String("config", "", "config file path")
	rootCmd.PersistentFlags().String("port", "8081", "HTTP port")
	rootCmd.PersistentFlags().String("log-level", "info", "log level")

	viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
	viper.BindPFlag("log_level", rootCmd.PersistentFlags().Lookup("log-level"))

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("failed to execute command")
	}
}

func run(cmd *cobra.Command, args []string) {
	// Setup logging
	setupLogging()

	// Load configuration
	if configFile := viper.GetString("config"); configFile != "" {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			log.Fatal().Err(err).Msg("failed to read config file")
		}
	}

	// Set defaults
	viper.SetDefault("database.url", "postgres://localhost/devtail?sslmode=disable")
	viper.SetDefault("hetzner.ssh_key_id", 0)
	viper.SetDefault("hetzner.network_id", 0)
	viper.SetDefault("gateway.url", "https://github.com/devtail/gateway/releases/latest/download/gateway-linux-amd64")
	viper.SetDefault("callback.url", "http://localhost:8081/api/v1/callbacks/vm")
	viper.SetDefault("websocket.base_url", "ws://localhost:8080")

	// Environment variables
	viper.AutomaticEnv()

	// Database connection
	db, err := sql.Open("postgres", viper.GetString("database.url"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}

	// Initialize clients
	hetznerClient := hetzner.NewClient(
		viper.GetString("hetzner.token"),
		viper.GetInt64("hetzner.ssh_key_id"),
		viper.GetInt64("hetzner.network_id"),
	)

	tailscaleClient := tailscale.NewClient(
		viper.GetString("tailscale.api_key"),
		viper.GetString("tailscale.tailnet"),
	)

	// Initialize VM manager
	vmManager := vm.NewManager(db, hetznerClient, tailscaleClient, vm.Config{
		SSHPublicKey:     viper.GetString("ssh.public_key"),
		GatewayURL:       viper.GetString("gateway.url"),
		CallbackURL:      viper.GetString("callback.url"),
		WebSocketBaseURL: viper.GetString("websocket.base_url"),
	})

	// Initialize handlers
	handlers := api.NewHandlers(vmManager)

	// Setup routes
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(ginLogger())

	// API routes
	v1 := router.Group("/api/v1")
	{
		v1.POST("/vms", handlers.CreateVM)
		v1.GET("/vms/:id", handlers.GetVM)
		v1.DELETE("/vms/:id", handlers.DeleteVM)
		v1.POST("/callbacks/vm", handlers.VMCallback)
	}

	router.GET("/health", handlers.HealthCheck)

	// Start server
	srv := &http.Server{
		Addr:    ":" + viper.GetString("port"),
		Handler: router,
	}

	go func() {
		log.Info().Str("port", viper.GetString("port")).Msg("starting control plane server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server shutdown failed")
	}
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level, err := zerolog.ParseLevel(viper.GetString("log_level"))
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	if os.Getenv("CONTROL_PLANE_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

func ginLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		log.Info().
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Str("ip", c.ClientIP()).
			Msg("request")
	}
}