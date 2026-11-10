package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jonace-mpelule/fQL/internal/cli"
)

// version is supplied by the build; direct go run invocations use dev.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr, version)
	stop()
	os.Exit(code)
}
