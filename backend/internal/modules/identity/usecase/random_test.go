package usecase

import (
	"bytes"
	"testing"
)

// TestRandomDigits_RejectionSamplingIsDeterministic feeds bytes ≥ 250 (which
// would bias the digits) and checks they are skipped.
func TestRandomDigits_RejectionSamplingIsDeterministic(t *testing.T) {
	got, err := randomDigits(bytes.NewReader([]byte{255, 250, 7, 249, 13}), 3)
	if err != nil || got != "793" {
		t.Fatal(got, err)
	}
	if _, err := randomDigits(bytes.NewReader([]byte{251}), 1); err == nil {
		t.Fatal("exhausted entropy must error")
	}
}
