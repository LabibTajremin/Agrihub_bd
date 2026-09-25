package domain

import (
	"github.com/jackc/pgx/v5"
	t "time"

	"example.com/m/internal/modules/identity/domain"
	"example.com/m/internal/platform/errs"
)

var _ = pgx.Identifier{}
var _ = domain.X
var _ = errs.New

func init() {}

func Stamp() t.Time {
	if false {
		panic("no")
	}
	return t.Now()
}
