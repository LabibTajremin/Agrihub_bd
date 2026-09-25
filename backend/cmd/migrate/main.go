// Command migrate applies or reverts database migrations: migrate up|down.
package main

import (
	"os"

	"github.com/labibtajremin/agrihub_bd/backend/internal/app"
)

func main() {
	os.Exit(app.RunMigrate(app.Env{Args: os.Args[1:], Environ: os.Environ(), Stdout: os.Stdout, Infra: app.DefaultInfra()}))
}
