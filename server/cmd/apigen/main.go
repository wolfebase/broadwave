package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	root := flag.String("root", ".", "repo root")
	check := flag.Bool("check", false, "exit 1 when generated files differ")
	flag.Parse()
	specPath := filepath.Join(*root, "api", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	schemas, order := schemasFrom(&doc)
	propOrder = order
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	swift := renderSwift(names, schemas)
	ts := renderTS(names, schemas)
	files := map[string]string{
		filepath.Join(*root, "apple/Packages/OTAKit/Sources/OTAKit/Generated.swift"): swift,
		filepath.Join(*root, "web/src/api/generated.ts"):                             ts,
	}
	if *check {
		bad := false
		for path, want := range files {
			got, err := os.ReadFile(path)
			if err != nil || string(got) != want {
				fmt.Fprintf(os.Stderr, "out of date: %s\n", path)
				bad = true
			}
		}
		if bad {
			os.Exit(1)
		}
		return
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

var propOrder = map[string][]string{}

func schemasFrom(doc *yaml.Node) (map[string]map[string]any, map[string][]string) {
	var decoded map[string]any
	if err := doc.Decode(&decoded); err != nil {
		return nil, nil
	}
	schemas := walk(decoded, "components", "schemas")
	order := map[string][]string{}
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	schemasNode := mapVal(mapVal(mapVal(root, "components"), "schemas"), "")
	_ = schemasNode
	if comps := mapVal(root, "components"); comps != nil {
		if block := mapVal(comps, "schemas"); block != nil {
			for i := 0; i+1 < len(block.Content); i += 2 {
				name := block.Content[i].Value
				order[name] = propNames(mapVal(block.Content[i+1], "properties"))
			}
		}
	}
	return schemas, order
}

func mapVal(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	if key == "" {
		return n
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func propNames(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	var names []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		names = append(names, n.Content[i].Value)
	}
	return names
}

func walk(doc map[string]any, keys ...string) map[string]map[string]any {
	var cur any = doc
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	out := map[string]map[string]any{}
	m, ok := cur.(map[string]any)
	if !ok {
		return out
	}
	for name, raw := range m {
		if schema, ok := raw.(map[string]any); ok {
			out[name] = schema
		}
	}
	return out
}

type field struct {
	name     string
	swift    string
	ts       string
	required bool
	date     bool
}

func fieldsOf(name string, schema map[string]any, schemas map[string]map[string]any) []field {
	props, _ := schema["properties"].(map[string]any)
	required := map[string]bool{}
	if list, ok := schema["required"].([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				required[s] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	if seq := propOrder[name]; len(seq) > 0 {
		rank := map[string]int{}
		for i, n := range seq {
			rank[n] = i
		}
		sort.Slice(names, func(i, j int) bool {
			ri, rj := rank[names[i]], rank[names[j]]
			if ri == rj {
				return names[i] < names[j]
			}
			return ri < rj
		})
	} else {
		sort.Strings(names)
	}
	var out []field
	for _, n := range names {
		prop, _ := props[n].(map[string]any)
		sw, ts, date := typ(name+export(n), prop, schemas)
		out = append(out, field{name: n, swift: sw, ts: ts, required: required[n], date: date})
	}
	return out
}

func typ(nested string, prop map[string]any, schemas map[string]map[string]any) (swift, ts string, date bool) {
	if prop == nil {
		return "String", "string", false
	}
	if ref, ok := prop["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		return name, name, false
	}
	if all, ok := prop["allOf"].([]any); ok && len(all) > 0 {
		if m, ok := all[0].(map[string]any); ok {
			return typ(nested, m, schemas)
		}
	}
	kind, _ := prop["type"].(string)
	switch kind {
	case "boolean":
		return "Bool", "boolean", false
	case "integer":
		if prop["format"] == "int64" {
			return "Int64", "number", false
		}
		return "Int", "number", false
	case "number":
		return "Double", "number", false
	case "array":
		items, _ := prop["items"].(map[string]any)
		if items != nil && items["type"] == "object" {
			sw, ts, _ := "String", "string", false
			sw, ts, _ = typ(nested, items, schemas)
			if items["type"] == "object" {
				return "[" + nested + "]", nested + "[]", false
			}
			return "[" + sw + "]", ts + "[]", false
		}
		sw, ts, _ := typ(nested, items, schemas)
		return "[" + sw + "]", ts + "[]", false
	case "object":
		return nested, nested, false
	default:
		if prop["format"] == "date-time" {
			return "Date", "string", true
		}
		return "String", "string", false
	}
}

func export(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func renderSwift(names []string, schemas map[string]map[string]any) string {
	var b bytes.Buffer
	b.WriteString("// Generated from api/openapi.yaml. Do not edit.\n\nimport Foundation\n\n")
	seen := map[string]bool{}
	var emit func(name string, schema map[string]any)
	emit = func(name string, schema map[string]any) {
		if seen[name] || schema["type"] == "string" && schema["properties"] == nil {
			if schema["type"] == "string" && schema["enum"] != nil && !seen[name] {
				seen[name] = true
				fmt.Fprintf(&b, "public typealias %s = String\n\n", name)
			}
			return
		}
		if schema["type"] != "object" && schema["properties"] == nil {
			return
		}
		props, _ := schema["properties"].(map[string]any)
		for propName, raw := range props {
			prop, _ := raw.(map[string]any)
			if prop != nil && prop["type"] == "object" {
				emit(name+export(propName), prop)
			}
			if prop != nil && prop["type"] == "array" {
				if items, ok := prop["items"].(map[string]any); ok && items["type"] == "object" {
					emit(name+export(propName), items)
				}
			}
		}
		seen[name] = true
		fields := fieldsOf(name, schema, schemas)
		ident := ""
		for _, f := range fields {
			if f.name == "id" {
				ident = ", Identifiable"
			}
		}
		fmt.Fprintf(&b, "public struct %s: Codable, Sendable, Hashable%s {\n", name, ident)
		for _, f := range fields {
			t := f.swift
			if !f.required {
				t += "?"
			}
			fmt.Fprintf(&b, "    public var %s: %s\n", f.name, t)
		}
		if len(fields) > 0 {
			b.WriteString("\n    public init(")
			for i, f := range fields {
				if i > 0 {
					b.WriteString(", ")
				}
				t := f.swift
				def := ""
				if !f.required {
					t += "?"
					def = " = nil"
				}
				fmt.Fprintf(&b, "%s: %s%s", f.name, t, def)
			}
			b.WriteString(") {\n")
			for _, f := range fields {
				fmt.Fprintf(&b, "        self.%s = %s\n", f.name, f.name)
			}
			b.WriteString("    }\n")
		}
		b.WriteString("}\n\n")
	}
	for _, name := range names {
		if name == "Prefs" || name == "Error" {
			continue
		}
		emit(name, schemas[name])
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func renderTS(names []string, schemas map[string]map[string]any) string {
	var b bytes.Buffer
	b.WriteString("// Generated from api/openapi.yaml. Do not edit.\n\n")
	for _, name := range names {
		if name == "Prefs" || name == "Error" {
			continue
		}
		schema := schemas[name]
		if schema["type"] == "string" && schema["properties"] == nil {
			if list, ok := schema["enum"].([]any); ok && len(list) > 0 {
				parts := make([]string, 0, len(list))
				for _, item := range list {
					parts = append(parts, fmt.Sprintf("%q", fmt.Sprint(item)))
				}
				fmt.Fprintf(&b, "export type %s = %s;\n\n", name, strings.Join(parts, " | "))
			} else {
				fmt.Fprintf(&b, "export type %s = string;\n\n", name)
			}
			continue
		}
		if schema["properties"] == nil && schema["allOf"] == nil {
			continue
		}
		merged := schema
		if all, ok := schema["allOf"].([]any); ok {
			merged = map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}}
			props := merged["properties"].(map[string]any)
			var req []any
			for _, item := range all {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if ref, ok := m["$ref"].(string); ok {
					base := ref[strings.LastIndex(ref, "/")+1:]
					if parent, ok := schemas[base]; ok {
						if p, ok := parent["properties"].(map[string]any); ok {
							for k, v := range p {
								props[k] = v
							}
						}
						if r, ok := parent["required"].([]any); ok {
							req = append(req, r...)
						}
					}
				}
				if p, ok := m["properties"].(map[string]any); ok {
					for k, v := range p {
						props[k] = v
					}
				}
			}
			merged["required"] = req
		}
		seen := map[string]bool{}
		var writeObj func(name string, schema map[string]any)
		writeObj = func(name string, schema map[string]any) {
			if seen[name] {
				return
			}
			props, _ := schema["properties"].(map[string]any)
			propNames := make([]string, 0, len(props))
			for n := range props {
				propNames = append(propNames, n)
			}
			sort.Strings(propNames)
			for _, n := range propNames {
				prop, _ := props[n].(map[string]any)
				if prop == nil {
					continue
				}
				if prop["type"] == "object" {
					writeObj(name+export(n), prop)
				}
				if prop["type"] == "array" {
					if items, ok := prop["items"].(map[string]any); ok && items["type"] == "object" {
						writeObj(name+export(n), items)
					}
				}
			}
			seen[name] = true
			fmt.Fprintf(&b, "export type %s = {\n", name)
			for _, f := range fieldsOf(name, schema, schemas) {
				opt := ""
				if !f.required {
					opt = "?"
				}
				fmt.Fprintf(&b, "  %s%s: %s;\n", f.name, opt, f.ts)
			}
			b.WriteString("};\n\n")
		}
		writeObj(name, merged)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
