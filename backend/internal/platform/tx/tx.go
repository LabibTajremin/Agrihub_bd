// Package tx declares the transaction boundary used by use cases. It carries no
// SQL types so use cases stay storage-agnostic.
package tx

import "context"

// Manager runs fn atomically. Nested calls join the outer transaction via a
// savepoint, so a failing inner unit rolls back alone.
type Manager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Nop runs fn directly; used by unit tests with in-memory fakes.
type Nop struct{}

// WithinTx implements Manager.
func (Nop) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }
