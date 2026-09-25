//go:build !serverless

// Command api serves the AgriSmart API (Docker / local target).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/labibtajremin/agrihub_bd/backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.RunAPI(ctx, app.Env{Args: os.Args[1:], Environ: os.Environ(), Stdout: os.Stdout, Infra: app.DefaultInfra()}, app.ServeOptions{})
	stop()
	os.Exit(code)
}
