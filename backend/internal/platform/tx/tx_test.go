package tx

import (
	"context"
	"errors"
	"testing"
)

func TestNop_RunsFn(t *testing.T) {
	want := errors.New("x")
	if err := (Nop{}).WithinTx(context.Background(), func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
}
