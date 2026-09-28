// Package registry selects the AI implementation by configuration
// (ai.provider). Only "stub" ships; a real model registers here — see
// docs/AI_INTEGRATION.md. Swapping providers is a config change only.
package registry

import (
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/stub"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// ErrUnknownProvider is returned for an unregistered provider name.
var ErrUnknownProvider = errs.Validation("ai.unknown_provider")

// Providers is the set of AI ports the rest of the system consumes.
type Providers struct {
	Diagnosis aiadapter.DiagnosisEngine
	Narrator  aiadapter.AdvisoryNarrator
	Agent     aiadapter.ConversationalAgent
}

// factories maps provider names to constructors (immutable table).
var factories = map[string]func() Providers{
	"stub": func() Providers {
		e := stub.Engine{}
		return Providers{Diagnosis: e, Narrator: e, Agent: e}
	},
}

// New returns the providers registered under name.
func New(name string) (Providers, error) {
	f, ok := factories[name]
	if !ok {
		return Providers{}, ErrUnknownProvider.WithField("ai.provider", name)
	}
	return f(), nil
}
