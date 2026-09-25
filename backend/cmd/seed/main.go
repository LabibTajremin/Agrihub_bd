// Command seed loads the bundled dictionaries into the database (idempotent).
package main

import (
	"context"
	"os"

	"github.com/labibtajremin/agrihub_bd/backend/internal/app"
)

func main() {
	os.Exit(app.RunSeed(context.Background(), app.Env{Args: os.Args[1:], Environ: os.Environ(), Stdout: os.Stdout, Infra: app.DefaultInfra()}))
}
