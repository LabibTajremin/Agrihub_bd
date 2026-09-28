// Package assistant exposes the voice assistant: a thin use case and HTTP
// endpoint over the ConversationalAgent port (stub by default). There is no
// STT/TTS: clients send a transcript (or chosen suggestion) and play the
// pre-recorded clip of the returned answer key.
package assistant

import (
	"context"
	"net/http"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Ask is the use case.
type Ask struct{ Agent aiadapter.ConversationalAgent }

// Execute authorises and forwards one turn.
func (uc Ask) Execute(ctx context.Context, in aiadapter.AskInput) (aiadapter.AskResult, error) {
	if _, err := authn.Require(ctx, authz.AssistantAsk); err != nil {
		return aiadapter.AskResult{}, err
	}
	return uc.Agent.Ask(ctx, in)
}

// AskRequest is one assistant turn.
type AskRequest struct {
	Lang         string            `json:"lang" validate:"required,max=5"`
	Transcript   string            `json:"transcript" validate:"max=500"`
	AudioMediaID string            `json:"audio_media_id" validate:"omitempty,uuid"`
	Context      map[string]string `json:"context"`
}

// AskResponse is the answer as dictionary keys.
type AskResponse struct {
	Understood bool              `json:"understood"`
	Intent     string            `json:"intent"`
	AnswerKey  string            `json:"answer_key"`
	Params     map[string]string `json:"params"`
	FollowUps  []string          `json:"follow_ups"`
}

// Module is the assembled assistant.
type Module struct {
	ask Ask
	v   *validator.Validator
}

// New wires the assistant over agent.
func New(agent aiadapter.ConversationalAgent, v *validator.Validator) *Module {
	return &Module{ask: Ask{Agent: agent}, v: v}
}

// Routes returns the assistant route.
func (m *Module) Routes() []httpx.Route {
	return []httpx.Route{{Method: http.MethodPost, Path: "/v1/assistant/ask", Permission: string(authz.AssistantAsk), Tag: "assistant",
		Summary: "Ask the voice assistant (stub agent; answer is a dictionary key)", Request: AskRequest{}, Response: AskResponse{}, Handler: m.handle}}
}

func (m *Module) handle(w http.ResponseWriter, r *http.Request) error {
	var in AskRequest
	if err := httpx.Decode(r, &in, m.v); err != nil {
		return err
	}
	res, err := m.ask.Execute(r.Context(), aiadapter.AskInput{Lang: in.Lang, Transcript: in.Transcript, AudioMediaID: in.AudioMediaID, Context: in.Context})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, AskResponse{Understood: res.Understood, Intent: res.Intent, AnswerKey: res.AnswerKey, Params: res.Params, FollowUps: res.FollowUps})
}

// Errors is the AI error catalogue.
func Errors() []*errs.Error { return aiadapter.Errors() }
