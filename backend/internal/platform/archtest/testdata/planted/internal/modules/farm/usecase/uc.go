package usecase

import (
	"github.com/jackc/pgx/v5"

	"example.com/m/internal/modules/aiadapter"
	"example.com/m/internal/platform/database"
)

var _ = pgx.Identifier{}
var _ = database.X
var _ = aiadapter.X
