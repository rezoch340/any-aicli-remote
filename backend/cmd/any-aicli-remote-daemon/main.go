package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
	"github.com/rezoch340/any-aicli-remote/backend/internal/server"
)

const (
	exitSuccess  = 0
	exitInternal = 1
	exitUsage    = 2
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(arguments []string, standardInput io.Reader, standardOutput, standardError io.Writer) int {
	if len(arguments) > 0 && arguments[0] == "config" {
		return runConfig(arguments[1:], standardInput, standardOutput, standardError)
	}
	if errorValue := runDaemon(arguments, standardError); errorValue != nil {
		if !errors.Is(errorValue, flag.ErrHelp) {
			fmt.Fprintln(standardError, "any-aicli-remote-daemon:", errorValue)
			return exitInternal
		}
	}
	return exitSuccess
}
func runDaemon(arguments []string, standardError io.Writer) error {
	executionContext, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runDaemonWithContext(executionContext, arguments, standardError)
}
func runDaemonWithContext(executionContext context.Context, arguments []string, standardError io.Writer) error {
	configuration, errorValue := config.Parse(arguments)
	if errorValue != nil {
		return errorValue
	}
	logger := slog.New(slog.NewTextHandler(standardError, &slog.HandlerOptions{Level: slog.LevelInfo}))
	daemon, errorValue := server.New(configuration, logger)
	if errorValue != nil {
		return errorValue
	}
	defer daemon.Close()
	logger.Info("Any AI CLI Remote daemon starting", "local", fmt.Sprintf("http://127.0.0.1:%d/", configuration.Port), "pair", fmt.Sprintf("http://127.0.0.1:%d/pair", configuration.Port), "runtime", configuration.RuntimeDirectory, "agent", fmt.Sprintf("%s:%d", configuration.AgentHost, configuration.AgentPort))
	if errorValue := daemon.Run(executionContext); errors.Is(errorValue, server.AlreadyRunningError) {
		logger.Info("healthy Any AI CLI Remote already owns the HTTP port; standing down", "port", configuration.Port)
		return nil
	}
	return errorValue
}
func runConfig(arguments []string, standardInput io.Reader, standardOutput, standardError io.Writer) int {
	if len(arguments) == 0 {
		usage(standardError)
		return exitUsage
	}
	if isHelp(arguments[0]) {
		usage(standardOutput)
		return exitSuccess
	}
	switch arguments[0] {
	case "show":
		if containsHelp(arguments[1:]) {
			usage(standardOutput)
			return exitSuccess
		}
		configuration, errorValue := config.ResolveNonSecretWithOutput(arguments[1:], standardError)
		if errorValue != nil {
			return reportConfigOperationError(standardError, errorValue)
		}
		encoded, errorValue := json.Marshal(configuration.Canonical)
		if errorValue != nil {
			fmt.Fprintln(standardError, "config show:", errorValue)
			return exitInternal
		}
		fmt.Fprintln(standardOutput, string(encoded))
		return exitSuccess
	case "validate":
		inputPath, remaining, errorValue := parseInput(arguments[1:])
		if errorValue != nil {
			return configError(standardError, errorValue)
		}
		if containsHelp(remaining) {
			usage(standardOutput)
			return exitSuccess
		}
		if inputPath == "" {
			_, errorValue = config.ResolveNonSecretWithOutput(remaining, standardError)
		} else {
			if errorValue = onlyConfig(remaining); errorValue == nil {
				_, errorValue = readCandidate(inputPath, standardInput)
			}
		}
		if errorValue != nil {
			if inputPath == "" {
				return reportConfigOperationError(standardError, errorValue)
			}
			return reportConfigOperationError(standardError, errorValue)
		}
		fmt.Fprintln(standardOutput, "valid")
		return exitSuccess
	case "apply":
		inputPath, remaining, errorValue := parseInput(arguments[1:])
		if errorValue != nil {
			return configError(standardError, errorValue)
		}
		if containsHelp(remaining) {
			usage(standardOutput)
			return exitSuccess
		}
		if inputPath == "" {
			return configError(standardError, errors.New("config apply requires --input FILE|-"))
		}
		if !hasExplicitConfig(remaining) {
			return configError(standardError, errors.New("config apply requires --config PATH"))
		}
		if errorValue = onlyConfig(remaining); errorValue != nil {
			return configError(standardError, errorValue)
		}
		configurationPath, errorValue := config.ResolveConfigurationPath(remaining)
		if errorValue != nil {
			return configError(standardError, errorValue)
		}
		document, errorValue := readCandidate(inputPath, standardInput)
		if errorValue != nil {
			return reportConfigOperationError(standardError, errorValue)
		}
		if errorValue = config.SaveDocument(configurationPath, document); errorValue != nil {
			fmt.Fprintln(standardError, "config apply:", errorValue)
			return exitInternal
		}
		fmt.Fprintln(standardOutput, configurationPath)
		return exitSuccess
	default:
		return configError(standardError, fmt.Errorf("unknown config command %q", arguments[0]))
	}
}
func readCandidate(inputPath string, standardInput io.Reader) (config.Document, error) {
	if inputPath == "-" {
		return config.DecodeAndNormalizeDocumentReader(standardInput)
	}
	file, errorValue := os.Open(inputPath)
	if errorValue != nil {
		return config.Document{}, errorValue
	}
	defer file.Close()
	return config.DecodeAndNormalizeDocumentReader(file)
}
func parseInput(arguments []string) (string, []string, error) {
	remaining := make([]string, 0, len(arguments))
	inputPath := ""
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--input" {
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return "", nil, errors.New("input path cannot be empty")
			}
			inputPath = arguments[index+1]
			index++
			continue
		}
		if strings.HasPrefix(argument, "--input=") {
			inputPath = strings.TrimPrefix(argument, "--input=")
			if strings.TrimSpace(inputPath) == "" {
				return "", nil, errors.New("input path cannot be empty")
			}
			continue
		}
		remaining = append(remaining, argument)
	}
	return inputPath, remaining, nil
}
func onlyConfig(arguments []string) error {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--config" || argument == "-config" {
			if index+1 >= len(arguments) {
				return errors.New("config path cannot be empty")
			}
			index++
			continue
		}
		if strings.HasPrefix(argument, "--config=") || strings.HasPrefix(argument, "-config=") {
			continue
		}
		return fmt.Errorf("candidate commands only accept --config")
	}
	return nil
}
func containsHelp(arguments []string) bool {
	for _, argument := range arguments {
		if isHelp(argument) {
			return true
		}
	}
	return false
}
func isHelp(argument string) bool { return argument == "--help" || argument == "-h" }
func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: any-aicli-remote-daemon config {show|validate|apply} [options]")
}
func hasExplicitConfig(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--config" || argument == "-config" || strings.HasPrefix(argument, "--config=") || strings.HasPrefix(argument, "-config=") {
			return true
		}
	}
	return false
}
func reportConfigOperationError(writer io.Writer, errorValue error) int {
	var pathError *os.PathError
	if errors.As(errorValue, &pathError) {
		fmt.Fprintln(writer, "config:", errorValue)
		return exitInternal
	}
	return configError(writer, errorValue)
}

func configError(writer io.Writer, errorValue error) int {
	fmt.Fprintln(writer, "config:", errorValue)
	return exitUsage
}
