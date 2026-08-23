package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
	"github.com/rezoch340/any-aicli-remote/backend/internal/server"
)

func main() {
	if errorValue := run(); errorValue != nil {
		fmt.Fprintln(os.Stderr, "any-aicli-remote-daemon:", errorValue)
		os.Exit(1)
	}
}

func run() error {
	configuration, errorValue := config.Parse(os.Args[1:])
	if errorValue != nil {
		if errors.Is(errorValue, flag.ErrHelp) {
			return nil
		}
		return errorValue
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	daemon, errorValue := server.New(configuration, logger)
	if errorValue != nil {
		return errorValue
	}
	defer daemon.Close()

	logger.Info("Any AI CLI Remote daemon starting",
		"local", fmt.Sprintf("http://127.0.0.1:%d/", configuration.Port),
		"pair", fmt.Sprintf("http://127.0.0.1:%d/pair", configuration.Port),
		"runtime", configuration.RuntimeDirectory,
		"agent", fmt.Sprintf("%s:%d", configuration.AgentHost, configuration.AgentPort),
	)
	executionContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if errorValue := daemon.Run(executionContext); errors.Is(errorValue, server.AlreadyRunningError) {
		logger.Info("healthy Any AI CLI Remote already owns the HTTP port; standing down", "port", configuration.Port)
		return nil
	} else {
		return errorValue
	}
}
