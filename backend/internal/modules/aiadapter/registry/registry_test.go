package registry

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	p, err := New("stub")
	if err != nil || p.Diagnosis == nil || p.Narrator == nil || p.Agent == nil {
		t.Fatal(err)
	}
	if _, err := New("gpt"); !errors.Is(err, ErrUnknownProvider) {
		t.Fatal(err)
	}
}
