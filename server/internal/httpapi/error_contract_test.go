package httpapi

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestErrorCodesAreNamedInTheSpec reads every machine code the handlers write
// and requires the Error schema to name it. The code stays a string so a new
// one does not break clients that already shipped.
func TestErrorCodesAreNamedInTheSpec(t *testing.T) {
	codes := codesWrittenByHandlers(t)
	if len(codes) < 23 {
		t.Fatalf("found %d codes, the scan missed some: %s", len(codes), strings.Join(codes, ", "))
	}
	doc := loadOpenAPI(t)
	schema, ok := asMap(doc.schemas["Error"])
	if !ok {
		t.Fatal("no Error schema")
	}
	props, _ := asMap(schema["properties"])
	codeProp, _ := asMap(props["code"])
	desc, _ := codeProp["description"].(string)
	if codeProp["enum"] != nil {
		t.Fatal("Error.code is an enum; an unknown code would fail to decode")
	}
	named := codeWords(desc)
	for _, code := range codes {
		if !named[code] {
			t.Errorf("Error.code description does not name %s", code)
		}
	}
	for _, key := range []string{"tiles", "limit", "retryAt"} {
		if _, exists := props[key]; !exists {
			t.Errorf("Error is missing %s", key)
		}
	}
}

// TestErrorBodiesMatchTheSchema starts the API and checks two envelopes the
// server writes without a tuner: a rejected body, and a guide refresh that
// comes too soon.
func TestErrorBodiesMatchTheSchema(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	if err := st.SetManualGuidePull(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	bad := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader("{"))
	bad.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	h.ServeHTTP(badRec, bad)
	assertErrorBody(t, doc, "put", "/settings", "400", badRec)

	limited := postJSON(t, h, "/api/v1/guide/refresh", "{}")
	body := assertErrorBody(t, doc, "post", "/guide/refresh", "429", limited)
	if body["code"] != "guide_rate_limited" {
		t.Fatalf("code %#v", body["code"])
	}
	if _, ok := body["retryAt"].(string); !ok {
		t.Fatalf("retryAt %#v", body["retryAt"])
	}
}

func assertErrorBody(t *testing.T, doc *apiDoc, method, path, status string, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	want, err := strconv.Atoi(status)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != want {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	schema, ok, err := responseSchema(doc, method, path, status)
	if err != nil || !ok {
		t.Fatalf("%s %s %s schema: ok=%v %v", method, path, status, ok, err)
	}
	if err := validateSchema(doc, schema, body, "$", 0); err != nil {
		t.Fatal(err)
	}
	return body
}

func codesWrittenByHandlers(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	add := func(expr ast.Expr) {
		text, ok := codeLiteral(expr)
		if ok {
			seen[text] = struct{}{}
		}
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Name == nil {
					continue
				}
				switch fn.Name.Name {
				case "codeFor":
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						ret, ok := n.(*ast.ReturnStmt)
						if ok && len(ret.Results) == 1 {
							add(ret.Results[0])
						}
						return true
					})
				case "errorFor":
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						ret, ok := n.(*ast.ReturnStmt)
						if ok && len(ret.Results) >= 2 {
							add(ret.Results[1])
						}
						return true
					})
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) < 3 {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if ok && id.Name == "apiError" {
					add(call.Args[2])
				}
				return true
			})
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func codeLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil || text == "" {
		return "", false
	}
	return text, true
}

func codeWords(desc string) map[string]bool {
	words := map[string]bool{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		words[b.String()] = true
		b.Reset()
	}
	for _, r := range desc {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words
}
