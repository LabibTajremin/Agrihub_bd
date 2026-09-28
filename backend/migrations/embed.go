// Package migrations embeds every SQL migration. Each module owns its own files
// (NNNNNN_<module>_<change>.{up,down}.sql) and its own tables.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
