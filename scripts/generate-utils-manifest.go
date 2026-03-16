package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type manifest struct {
	Version   int            `json:"version"`
	Utilities []manifestUtil `json:"utilities"`
}

type manifestUtil struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Binary  string `json:"binary"`
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}

	data, err := generateManifest(root)
	if err != nil {
		fail(err)
	}

	outputPath := filepath.Join(root, "manifest", "utils.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		fail(err)
	}
}

func generateManifest(root string) ([]byte, error) {
	cmdRoot := filepath.Join(root, "cmd")
	entries, err := os.ReadDir(cmdRoot)
	if err != nil {
		return nil, err
	}

	var utilities []manifestUtil
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		summary, err := packageSummary(filepath.Join(cmdRoot, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		utilities = append(utilities, manifestUtil{
			Name:    name,
			Summary: summary,
			Binary:  name,
		})
	}

	sort.Slice(utilities, func(i, j int) bool {
		return utilities[i].Name < utilities[j].Name
	})

	data, err := json.MarshalIndent(manifest{
		Version:   1,
		Utilities: utilities,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func packageSummary(dir string) (string, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return "", err
	}

	pkg, ok := pkgs["main"]
	if !ok {
		return "", fmt.Errorf("main package not found")
	}

	var files []*ast.File
	for _, file := range pkg.Files {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return fset.Position(files[i].Package).Filename < fset.Position(files[j].Package).Filename
	})

	for _, file := range files {
		if file.Doc == nil {
			continue
		}
		summary := normalizeSummary(strings.TrimSpace(firstParagraph(file.Doc.Text())))
		if summary != "" {
			return summary, nil
		}
	}

	return "", fmt.Errorf("package doc comment not found")
}

func firstParagraph(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(text, "\n\n")
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(strings.Fields(parts[0]), " ")
}

func normalizeSummary(summary string) string {
	if summary == "" {
		return ""
	}

	fields := strings.Fields(summary)
	if len(fields) >= 3 && fields[0] == "Command" {
		return strings.Join(fields[2:], " ")
	}

	return summary
}

func fail(err error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "generate-utils-manifest: %v\n", err)
	_, _ = os.Stderr.Write(buf.Bytes())
	os.Exit(1)
}
