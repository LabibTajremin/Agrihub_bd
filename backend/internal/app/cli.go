package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/migrations"
)

// Exit codes.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitConfig = 2
)

// Env is what an entrypoint receives from the process.
type Env struct {
	Args    []string
	Environ []string
	Stdout  io.Writer
	Infra   Infra
}

func loadConfig(env Env, args []string) (*config.Config, int) {
	cfg, err := config.Load(config.Source{Args: args, Environ: env.Environ, ReadFile: os.ReadFile})
	if err != nil {
		fmt.Fprintln(env.Stdout, config.Report(err))
		return nil, ExitConfig
	}
	return cfg, ExitOK
}

func loadConfigErr(env Env) (*config.Config, error) {
	return config.Load(config.Source{Environ: env.Environ})
}

// RunMigrate applies ("up") or reverts ("down") all migrations: migrate up|down [flags].
func RunMigrate(env Env) int {
	if len(env.Args) == 0 || (env.Args[0] != "up" && env.Args[0] != "down") {
		fmt.Fprintln(env.Stdout, "usage: migrate up|down [-config file] [-set key=value]")
		return ExitConfig
	}
	cfg, code := loadConfig(env, env.Args[1:])
	if cfg == nil {
		return code
	}
	if err := database.Migrate(cfg.Database.URL, migrations.FS, env.Args[0]); err != nil {
		fmt.Fprintln(env.Stdout, "migrate:", err)
		return ExitFailed
	}
	fmt.Fprintln(env.Stdout, "migrate", env.Args[0], "ok")
	return ExitOK
}

// RunSeed loads the bundled dictionaries (idempotent).
func RunSeed(ctx context.Context, env Env) int {
	cfg, code := loadConfig(env, env.Args)
	if cfg == nil {
		return code
	}
	a, err := Build(ctx, cfg, env.Infra)
	if err != nil {
		fmt.Fprintln(env.Stdout, "seed:", err)
		return ExitFailed
	}
	defer a.Close()
	changed, err := a.Localization.SeedBundled(ctx)
	if err != nil {
		fmt.Fprintln(env.Stdout, "seed:", err)
		return ExitFailed
	}
	for _, lang := range []string{"bn", "en", "hi", "es", "fr", "ar", "pt"} {
		fmt.Fprintf(env.Stdout, "seed %s: %d entries changed\n", lang, changed[lang])
	}
	return ExitOK
}
