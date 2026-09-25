package app

import (
	"context"
	"net/http"
)

// NewHandler builds the handler tree for the serverless target from the
// process environment (no config file, no flags, no background workers).
func NewHandler(ctx context.Context, environ []string, in Infra) (http.Handler, error) {
	env := Env{Environ: environ, Infra: in}
	cfg, err := loadConfigErr(env)
	if err != nil {
		return nil, err
	}
	a, err := Build(ctx, cfg, in)
	if err != nil {
		return nil, err
	}
	return a.Router, nil
}
