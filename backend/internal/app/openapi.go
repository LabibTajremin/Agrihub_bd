package app

import (
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
)

// OpenAPI generates the OpenAPI 3 document from the route table, so the spec
// is derived from code and can never drift (TestOpenAPI_UpToDate diffs it).
func OpenAPI(routes []httpx.Route) ([]byte, error) {
	g := &gen{schemas: map[string]any{}}
	paths := map[string]map[string]any{}
	for _, r := range routes {
		path := strings.ReplaceAll(r.Path, "...}", "}")
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		paths[path][strings.ToLower(r.Method)] = g.operation(r, path)
	}
	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{"title": "AgriSmart API", "version": "1.0.0",
			"description": "Offline-first farming assistant API. Error `message` values are i18n dictionary keys."},
		"paths": paths,
		"components": map[string]any{
			"schemas":         g.schemas,
			"securitySchemes": map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
		},
	}
	return yaml.Marshal(doc)
}

type gen struct{ schemas map[string]any }

var pathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

func (g *gen) operation(r httpx.Route, path string) map[string]any {
	op := map[string]any{"summary": r.Summary, "tags": []string{r.Tag},
		"operationId": strings.ToLower(r.Method) + strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace(path)}
	var params []any
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		params = append(params, map[string]any{"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
	}
	if r.Query != nil {
		t := reflect.TypeOf(r.Query)
		for i := range t.NumField() {
			f := t.Field(i)
			params = append(params, map[string]any{"name": f.Tag.Get("query"), "in": "query", "required": false, "schema": g.schema(f.Type)})
		}
	}
	if len(params) > 0 {
		op["parameters"] = params
	}
	if r.Request != nil {
		op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(r.Request))}}}
	}
	status := r.Status
	if status == 0 {
		status = 200
	}
	ok := map[string]any{"description": "success"}
	if r.Response != nil {
		ok["content"] = map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(r.Response))}}
	}
	op["responses"] = map[string]any{
		strconv.Itoa(status): ok,
		"default": map[string]any{"description": "error", "content": map[string]any{"application/json": map[string]any{
			"schema": g.schema(reflect.TypeOf(httpx.ErrorBody{}))}}},
	}
	if r.Permission != "" {
		op["security"] = []any{map[string]any{"bearer": []string{}}}
		op["x-permission"] = r.Permission
	}
	return op
}

var timeType = reflect.TypeOf(time.Time{})

// componentName qualifies a struct by its module (or platform package).
func componentName(t reflect.Type) string {
	parts := strings.Split(t.PkgPath(), "/")
	owner := parts[len(parts)-1]
	if len(parts) >= 2 && parts[len(parts)-1] == "transport" {
		owner = parts[len(parts)-2]
	}
	return owner + "." + t.Name()
}

func (g *gen) schema(t reflect.Type) map[string]any {
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t.Kind() == reflect.Pointer:
		s := g.schema(t.Elem())
		if _, isRef := s["$ref"]; isRef {
			return map[string]any{"allOf": []any{s}, "nullable": true}
		}
		s["nullable"] = true
		return s
	case t.Kind() == reflect.Struct:
		name := componentName(t)
		if _, done := g.schemas[name]; !done {
			g.schemas[name] = map[string]any{} // reserve (recursion guard)
			g.schemas[name] = g.object(t)
		}
		return map[string]any{"$ref": "#/components/schemas/" + name}
	case t.Kind() == reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case t.Kind() == reflect.Slice:
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case t.Kind() == reflect.String:
		return map[string]any{"type": "string"}
	case t.Kind() == reflect.Bool:
		return map[string]any{"type": "boolean"}
	case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
		return map[string]any{"type": "number"}
	default: // all integer kinds
		return map[string]any{"type": "integer", "format": "int64"}
	}
}

func (g *gen) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []string
	g.fields(t, props, &required)
	sort.Strings(required)
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func (g *gen) fields(t reflect.Type, props map[string]any, required *[]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			g.fields(f.Type, props, required)
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" || name == "" {
			continue
		}
		props[name] = g.schema(f.Type)
		if strings.Contains(f.Tag.Get("validate"), "required") {
			*required = append(*required, name)
		}
	}
}
