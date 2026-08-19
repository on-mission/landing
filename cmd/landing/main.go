package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/on-mission/landing/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code, err := cli.Run(ctx, cli.Inputs{
		Args:            os.Args[1:],
		Stdin:           os.Stdin,
		StdinIsTerminal: cli.IsTerminalStdin(os.Stdin),
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	})
	if err != nil {
		code = cli.ReportError(os.Stderr, err)
	}
	os.Exit(code)
}
