package validator

import (
	"errors"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

type inner struct {
	Key string `json:"key" validate:"i18nkey"`
}

type sample struct {
	Name  string `json:"name" validate:"required"`
	Age   int    `yaml:"age" validate:"min=1"`
	Plain string `validate:"max=2"`
	Skip  string `json:"-" validate:"omitempty,len=3"`
	In    inner  `json:"in"`
}

func TestStruct_ReportsEachFieldByWireName(t *testing.T) {
	v := New()
	err := v.Struct(sample{Plain: "long", Skip: "ab", In: inner{Key: "Bad Key"}})
	e, ok := errs.As(err)
	if !ok || !errors.Is(err, ErrInvalid) {
		t.Fatalf("want validation error, got %v", err)
	}
	want := map[string]string{"name": "required", "age": "min=1", "Plain": "max=2", "Skip": "len=3", "in.key": "i18nkey"}
	for k, d := range want {
		if e.Fields[k] != d {
			t.Errorf("%s: want %q got %q (%v)", k, d, e.Fields[k], e.Fields)
		}
	}
}

func TestStruct_Valid(t *testing.T) {
	if err := New().Struct(sample{Name: "a", Age: 2, In: inner{Key: "home.title"}}); err != nil {
		t.Fatal(err)
	}
}

func TestStruct_NonStructIsInvalid(t *testing.T) {
	if err := New().Struct(42); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}
