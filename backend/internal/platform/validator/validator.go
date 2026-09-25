// Package validator validates structs by their `validate` tags and reports
// failures as a typed validation error with one entry per field.
package validator

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// ErrInvalid is returned when a struct fails validation.
var ErrInvalid = errs.Validation("request.invalid")

// KeyPattern is the i18n key format (screen.section.element).
var KeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Validator wraps go-playground/validator configured to report json/yaml names.
type Validator struct {
	v *validator.Validate
}

// New builds a Validator with the project's custom rules registered.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(fieldName)
	_ = v.RegisterValidation("i18nkey", func(fl validator.FieldLevel) bool {
		return KeyPattern.MatchString(fl.Field().String())
	})
	return &Validator{v: v}
}

func fieldName(f reflect.StructField) string {
	for _, tag := range []string{"json", "yaml", "query"} {
		name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
		if name != "" && name != "-" {
			return name
		}
	}
	return f.Name
}

// Struct validates s; nil when valid.
func (v *Validator) Struct(s any) error {
	err := v.v.Struct(s)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return ErrInvalid.Wrap(err)
	}
	out := ErrInvalid
	for _, fe := range verrs {
		ns := fe.Namespace()
		if i := strings.IndexByte(ns, '.'); i >= 0 {
			ns = ns[i+1:]
		}
		detail := fe.Tag()
		if fe.Param() != "" {
			detail += "=" + fe.Param()
		}
		out = out.WithField(ns, detail)
	}
	return out
}
