package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	outFlag := flag.String("out", "", "directory for the static site (default: docs/sitegen/dist)")
	docsFlag := flag.String("docs", "", "docs directory (default: found from the working directory)")
	flag.Parse()
	docs := *docsFlag
	if docs == "" {
		found, err := findDocs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "docs: %s\n", err)
			os.Exit(1)
		}
		docs = found
	}
	out := *outFlag
	if out == "" {
		out = filepath.Join(docs, "sitegen", "dist")
	}
	if err := Build(docs, out); err != nil {
		fmt.Fprintf(os.Stderr, "docs: %s\n", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

// findDocs walks up from the working directory to a docs folder that holds guide/index.md.
func findDocs() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if filepath.Base(dir) == "docs" {
			if _, err := os.Stat(filepath.Join(dir, "guide", "index.md")); err == nil {
				return dir, nil
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "docs", "guide", "index.md")); err == nil {
			return filepath.Join(dir, "docs"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find docs/guide/index.md from the working directory")
		}
		dir = parent
	}
}
