// Package handler is the Vercel serverless entrypoint. It exports the very
// same handler tree as cmd/api; the app is built once per cold start. Build
// with -tags serverless (vercel.json) so the server/worker code is compiled out.
package handler

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/labibtajremin/agrihub_bd/backend/internal/app"
)

var (
	once    sync.Once
	handler http.Handler
	initErr error
)

// Handler is invoked by the Vercel Go runtime for every request.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(func() { handler, initErr = app.NewHandler(context.Background(), os.Environ(), app.DefaultInfra()) })
	if initErr != nil {
		http.Error(w, `{"error":{"code":"internal","message":"errors.internal"}}`, http.StatusInternalServerError)
		return
	}
	handler.ServeHTTP(w, r)
}
