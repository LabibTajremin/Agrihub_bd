package app

import (
	"sort"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/assistant"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// ErrorCatalogue lists every error a client can receive, sorted by code. Each
// message key must exist in every dictionary (asserted by tests).
func ErrorCatalogue() []*errs.Error {
	all := []*errs.Error{
		errs.Internal("internal"), errs.Internal("panic"), database.ErrUnavailable, validator.ErrInvalid,
		httpx.ErrMalformed, httpx.ErrTooLarge, httpx.ErrBadParam, httpx.ErrRateLimited, httpx.ErrRouteNotFound,
		authn.ErrRequired, authn.ErrInvalidToken, authz.ErrForbidden,
	}
	for _, list := range [][]*errs.Error{identity.Errors(), localization.Errors(), farm.Errors(), media.Errors(),
		diagnosis.Errors(), advisory.Errors(), weather.Errors(), alert.Errors(), assistant.Errors()} {
		all = append(all, list...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Code < all[j].Code })
	return all
}
