// Command cadenza-server runs the Cadenza server: the API, the web app,
// notifications and MCP for cloud agents. Configuration is read from
// CADENZA_* environment variables; see docs/self-hosting.md.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mutex-solutions/cadenza/internal/server"
)

// Version is set at build time with -ldflags "-X main.Version=…".
var Version = "dev"

const usage = `cadenza-server: the Cadenza server.

Usage:
  cadenza-server [serve]                 run the server (default)
  cadenza-server owner reset-password    set the owner password (reads it from stdin)
  cadenza-server version                 print the version

Configuration comes from CADENZA_* environment variables, e.g.
  CADENZA_PUBLIC_URL=https://cadenza.example.com
  CADENZA_DATABASE=postgres://user:pass@db/cadenza   (default: sqlite in CADENZA_DATA_DIR)
  CADENZA_S3_BUCKET, CADENZA_S3_ENDPOINT, …           (default: files on local disk, trial mode)
See docs/self-hosting.md for the full list.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cadenza-server:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("cadenza-server", Version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	case "serve":
		cfg, err := server.LoadConfig(os.Getenv)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		s, err := server.New(ctx, cfg, Version, log)
		if err != nil {
			return err
		}
		defer s.Close()
		return s.Run(ctx)
	case "owner":
		if len(args) < 2 || args[1] != "reset-password" {
			return fmt.Errorf("usage: cadenza-server owner reset-password")
		}
		cfg, err := server.LoadConfig(os.Getenv)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, "New owner password: ")
		pw, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && pw == "" {
			return fmt.Errorf("read password: %w", err)
		}
		ctx := context.Background()
		s, err := server.New(ctx, cfg, Version, log)
		if err != nil {
			return err
		}
		defer s.Close()
		if err := s.ResetPassword(ctx, strings.TrimRight(pw, "\r\n")); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Owner password set.")
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}
