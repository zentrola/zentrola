package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

type commandOptions struct {
	name, username, configPath string
	help                       bool
	configProvided             bool
	port                       int
}

func knownCommand(name string) bool {
	switch name {
	case "serve", "start", "restart", "stop", "status", "migrate", "healthcheck", "password", "help":
		return true
	}
	return false
}

func parseCommand(args []string) (commandOptions, error) {
	result := commandOptions{name: "serve", configPath: ".env"}
	invalid := errors.New("invalid command or arguments; run 'zentrola help'")
	if len(args) == 0 {
		return result, nil
	}
	result.name = args[0]
	if result.name == "--help" || result.name == "-h" {
		result.name = "help"
	}
	if !knownCommand(result.name) {
		return result, invalid
	}
	if result.name == "help" {
		result.help = true
		if len(args) == 2 && knownCommand(args[1]) {
			result.name = args[1]
		} else if len(args) > 1 {
			return result, invalid
		}
		return result, nil
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		result.help = true
		return result, nil
	}
	flags := flag.NewFlagSet(result.name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configSeen := false
	if result.name != "stop" && result.name != "status" {
		flags.Func("config", "config file (default: .env)", func(value string) error {
			if configSeen || strings.TrimSpace(value) == "" {
				return invalid
			}
			configSeen = true
			result.configProvided = true
			result.configPath = value
			return nil
		})
	}
	if result.name == "start" || result.name == "restart" || result.name == "serve" {
		flags.Func("port", "port for this run (1-65535)", func(value string) error {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 65535 || result.port != 0 {
				return invalid
			}
			result.port = n
			return nil
		})
	}
	if result.name == "password" {
		seen := false
		flags.Func("username", "administrator username", func(value string) error {
			if seen {
				return invalid
			}
			seen = true
			result.username = value
			return nil
		})
	}
	if err := flags.Parse(args[1:]); err != nil {
		return result, invalid
	}
	if flags.NArg() != 0 {
		return result, invalid
	}
	if result.name == "password" && !appsec.ValidAdminUsername(result.username) {
		return result, errors.New("usage: zentrola password --username <name>; name must be 1-64 bytes with no control characters or surrounding whitespace")
	}
	return result, nil
}

func loadCommandConfig(path string) (config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		resolved, pathErr := filepath.Abs(path)
		if pathErr != nil {
			resolved = path
		}
		return config.Config{}, fmt.Errorf("cannot load config %q: %w; use --config <path> to select a file", resolved, err)
	}
	return cfg, nil
}

func writeHelp(output io.Writer, name string) error {
	if name == "start" || name == "restart" || name == "stop" || name == "status" || name == "serve" {
		_, err := fmt.Fprintln(output, `Usage: zentrola start [--port <port>] [--config <path>]
       zentrola restart [--port <port>] [--config <path>]
       zentrola stop
       zentrola status

start runs in the background. restart reuses the last config and port.
stop shuts down gracefully. status shows the current state.
serve runs in the foreground and is the default when no command is given.`)
		return err
	}
	if name == "healthcheck" {
		_, err := fmt.Fprintln(output, `Usage: zentrola healthcheck [--config <path>]

Checks /health/live and prints the response. Uses the managed server when available,
otherwise HTTP_ADDR (default: :9527) or the address from --config.`)
		return err
	}
	if name == "password" {
		_, err := fmt.Fprintln(output, `Usage: zentrola password --username <name> [--config <path>]

Generates a new password, clears the login lock, and invalidates old tokens.
The password is shown once. Run this command in a trusted terminal.`)
		return err
	}
	_, err := fmt.Fprintln(output, `Usage: zentrola [command]

Commands:
  start [--port <port>]          Start in the background
  restart [--port <port>]        Restart with the last settings
  stop                           Stop gracefully
  status                         Show server status
  serve                          Run in the foreground (default)
  migrate                        Run database migrations
  healthcheck                    Check server health
  password --username <name>     Reset an administrator password
  help [command]                 Show help

Use --config <path> to select a config file. The default is .env.`)
	return err
}

func resetPassword(ctx context.Context, output io.Writer, username string, reset func(context.Context, string) (string, error)) error {
	password, err := reset(ctx, username)
	if errors.Is(err, appsec.ErrNotFound) {
		return errors.New("active administrator not found; check the username and organization status")
	}
	if err != nil {
		return errors.New("password reset failed; check the database, migrations, and audit log")
	}
	if _, err := fmt.Fprintf(output, "Password reset for %s. Login lock cleared and old tokens invalidated.\nNew password (shown once): %s\nSave it in a password manager.\n", username, password); err != nil {
		return errors.New("password was reset, but the result could not be displayed; run the reset again in a working terminal")
	}
	return nil
}
