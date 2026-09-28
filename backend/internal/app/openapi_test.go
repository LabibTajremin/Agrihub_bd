package app

import (
	"strings"
	"testing"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
)

type node struct {
	Name     string  `json:"name" validate:"required"`
	Next     *node   `json:"next"`
	Weight   float32 `json:"weight"`
	Count    *int    `json:"count"`
	Skip     string  `json:"-"`
	Untagged string
	hidden   string
}

func TestOpenAPI_SchemaShapes(t *testing.T) {
	doc, err := OpenAPI([]httpx.Route{{Method: "POST", Path: "/x/{id}/{rest...}", Tag: "t", Request: node{}, Status: 204}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	for _, want := range []string{"app.node:", "nullable: true", "type: number", "required:\n                - name", "/x/{id}/{rest}:", "\"204\":"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	for _, unwanted := range []string{"Skip", "Untagged", "hidden"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("%s must not be documented", unwanted)
		}
	}
	_ = node{hidden: ""}.hidden
}
