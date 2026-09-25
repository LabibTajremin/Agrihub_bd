package kernel

import (
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
)

func TestFactory(t *testing.T) {
	k := Kernel{Clock: clock.System{}, IDs: &idgen.Sequence{}}
	if f := k.Factory(); f.IDs == nil || f.Clock == nil {
		t.Fatal()
	}
}
