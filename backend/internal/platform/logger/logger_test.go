package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{"debug": slog.LevelDebug, "WARN": slog.LevelWarn, "error": slog.LevelError, "info": slog.LevelInfo, "": slog.LevelInfo} {
		if ParseLevel(in) != want {
			t.Errorf("%s", in)
		}
	}
}

func TestNew_JSONIncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "info", "json").With("svc", "api").WithGroup("g")
	ctx := WithRequestID(context.Background(), "rid-1")
	l.InfoContext(ctx, "hello", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err, buf.String())
	}
	if rec["svc"] != "api" || rec["msg"] != "hello" {
		t.Fatal(rec)
	}
	if g, _ := rec["g"].(map[string]any); g["request_id"] != "rid-1" {
		t.Fatal(rec)
	}
	if RequestID(context.Background()) != "" {
		t.Fatal("empty ctx")
	}
}

func TestNew_TextWithoutRequestID(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, "debug", "text").Debug("x")
	if !strings.Contains(buf.String(), "msg=x") || strings.Contains(buf.String(), "request_id") {
		t.Fatal(buf.String())
	}
	Discard().Info("dropped")
}

func TestHashPII_StableAndOpaque(t *testing.T) {
	a := HashPII("+8801711000000")
	if a != HashPII("+8801711000000") || len(a) != 12 || strings.Contains(a, "1711") {
		t.Fatal(a)
	}
}
