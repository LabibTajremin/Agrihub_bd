//go:build ignore

// coverage_gate enforces the per-package coverage threshold. Logic lives in
// internal/tools/covgate so it is itself covered by tests.
package main

import (
	"os"

	"github.com/labibtajremin/agrihub_bd/backend/internal/tools/covgate"
)

func main() {
	os.Exit(covgate.Run(os.Args[1:], os.DirFS("."), os.Stdout))
}
