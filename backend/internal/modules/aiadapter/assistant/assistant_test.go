package assistant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/assistant"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/stub"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
)

func TestAsk_OverHTTP(t *testing.T) {
	a := apitest.New(t, nil)
	a.Mount(assistant.New(stub.Engine{}, a.Kernel.Validator).Routes())
	tok := a.Token("00000000-0000-7000-8000-0000000000a1", authz.Guest)
	var res assistant.AskResponse
	a.Do("POST", "/v1/assistant/ask", assistant.AskRequest{Lang: "bn", Transcript: "এই সপ্তাহে কি বৃষ্টি হবে?"}, tok).Expect(t, 200).Decode(t, &res)
	if !res.Understood || res.Intent != "weather" || res.AnswerKey != "voice.answer.weather" || len(res.FollowUps) != 2 {
		t.Fatalf("%+v", res)
	}
	a.Do("POST", "/v1/assistant/ask", assistant.AskRequest{Lang: "bn", Transcript: "hmm"}, tok).Expect(t, 200).Decode(t, &res)
	if res.Understood || res.AnswerKey != "voice.not_understood" {
		t.Fatal(res)
	}
	if c := a.Do("POST", "/v1/assistant/ask", assistant.AskRequest{Lang: "bn"}, tok).Expect(t, 400).ErrorCode(); c != "ai.invalid_input" {
		t.Fatal(c)
	}
	a.Do("POST", "/v1/assistant/ask", "{", tok).Expect(t, 400)
	a.Do("POST", "/v1/assistant/ask", assistant.AskRequest{Lang: "bn", Transcript: "x"}, "").Expect(t, 401)
	if _, err := (assistant.Ask{Agent: stub.Engine{}}).Execute(context.Background(), aiadapter.AskInput{}); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if len(assistant.Errors()) != 2 {
		t.Fatal()
	}
}
