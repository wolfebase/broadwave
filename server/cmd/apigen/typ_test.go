package main

import "testing"

func TestTypUnwrapsNullableRef(t *testing.T) {
	prop := map[string]any{
		"anyOf": []any{
			map[string]any{"$ref": "#/components/schemas/ClientDevice"},
			map[string]any{"type": "null"},
		},
	}
	swift, ts, date := typ("", prop, nil)
	if swift != "ClientDevice" || ts != "ClientDevice" || date {
		t.Fatalf("nullable ref %s %s %v", swift, ts, date)
	}

	swapped := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "null"},
			map[string]any{"$ref": "#/components/schemas/ClientDevice"},
		},
	}
	swift, ts, date = typ("", swapped, nil)
	if swift != "ClientDevice" || ts != "ClientDevice" || date {
		t.Fatalf("swapped %s %s %v", swift, ts, date)
	}

	twoRefs := map[string]any{
		"anyOf": []any{
			map[string]any{"$ref": "#/components/schemas/A"},
			map[string]any{"$ref": "#/components/schemas/B"},
		},
	}
	swift, ts, _ = typ("", twoRefs, nil)
	if swift != "String" || ts != "string" {
		t.Fatalf("two refs %s %s", swift, ts)
	}
}
