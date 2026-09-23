package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryRouteIsInOpenAPI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	path := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":") {
			path = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "     ") {
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			path = ""
			continue
		}
		method := strings.TrimSuffix(strings.TrimSpace(line), ":")
		switch method {
		case "get", "post", "put", "patch", "delete":
			if path != "" {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	s := &Server{}
	s.Handler()
	if len(s.routes) == 0 {
		t.Fatal("no routes registered")
	}
	for _, route := range s.routes {
		if !documented[route] {
			t.Errorf("%s is served but missing from api/openapi.yaml", route)
		}
	}
}
