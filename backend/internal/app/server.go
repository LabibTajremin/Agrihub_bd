//go:build !serverless

package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// This file is compiled out of the Vercel build (-tags serverless): the HTTP
// server, graceful shutdown and the background outbox worker exist only on the
// long-running Docker target.

// ServeOptions tunes RunAPI for tests.
type ServeOptions struct {
	// Ready is called with the bound listener once the server accepts connections.
	Ready func(ln net.Listener)
}

// RunAPI loads config, builds the app and serves until ctx is cancelled.
func RunAPI(ctx context.Context, env Env, o ServeOptions) int {
	cfg, code := loadConfig(env, env.Args)
	if cfg == nil {
		return code
	}
	a, err := Build(ctx, cfg, env.Infra)
	if err != nil {
		fmt.Fprintln(env.Stdout, "api:", err)
		return ExitFailed
	}
	defer a.Close()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		fmt.Fprintln(env.Stdout, "api:", err)
		return ExitFailed
	}
	if o.Ready != nil {
		o.Ready(ln)
	}
	if err := a.Serve(ctx, ln); err != nil {
		fmt.Fprintln(env.Stdout, "api:", err)
		return ExitFailed
	}
	return ExitOK
}

// Serve runs the HTTP server and the outbox worker until ctx is cancelled,
// then shuts down gracefully within http.shutdown_timeout.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	c := a.Config.HTTP
	srv := &http.Server{Handler: a.Router, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout,
		ReadHeaderTimeout: c.ReadTimeout, BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) }}
	ticker := time.NewTicker(a.Config.Database.OutboxPollInterval)
	defer ticker.Stop()
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		a.runOutboxWorker(workerCtx, ticker.C)
	}()
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	a.Logger.InfoContext(ctx, "api listening", "addr", ln.Addr().String())
	var err error
	select {
	case err = <-errc:
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.ShutdownTimeout)
		defer cancel()
		err = srv.Shutdown(shutdownCtx)
	}
	stopWorker()
	<-workerDone
	return err
}

// runOutboxWorker relays pending events on every tick (Docker target only).
func (a *App) runOutboxWorker(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if _, err := a.Relay.Flush(ctx); err != nil && ctx.Err() == nil {
				a.Logger.WarnContext(ctx, "outbox worker flush failed", "error", err)
			}
		}
	}
}
