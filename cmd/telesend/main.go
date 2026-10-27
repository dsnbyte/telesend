package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dsnbyte/telesend/internal/app"
	"github.com/dsnbyte/telesend/internal/cli"
	"github.com/dsnbyte/telesend/internal/envfile"
	"github.com/dsnbyte/telesend/internal/paths"
)

func main() {
	_ = envfile.Load(".env")

	p, err := paths.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	a, err := app.New(p.DatabaseFile, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer a.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	code := cli.Run(ctx, os.Args[1:], a, cli.DefaultIO())
	os.Exit(code)
}
