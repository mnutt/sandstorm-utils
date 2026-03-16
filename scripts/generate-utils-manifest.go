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
	"strconv"
	"strings"
)

type manifest struct {
	Version   int            `json:"version"`
	Utilities []manifestUtil `json:"utilities"`
}

type manifestUtil struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
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
		meta, err := commandMetadata(filepath.Join(cmdRoot, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		utilities = append(utilities, manifestUtil{
			Name:        name,
			Summary:     meta.summary,
			Description: meta.description,
			Examples:    meta.examples,
		})
	}

	sort.Slice(utilities, func(i, j int) bool {
		return utilities[i].Name < utilities[j].Name
	})

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(manifest{
		Version:   1,
		Utilities: utilities,
	})
	if err != nil {
		return nil, err
	}
	data := buf.Bytes()
	data = bytes.ReplaceAll(data, []byte(`\u003c`), []byte("<"))
	data = bytes.ReplaceAll(data, []byte(`\u003e`), []byte(">"))
	data = bytes.ReplaceAll(data, []byte(`\u0026`), []byte("&"))
	return data, nil
}

type commandMeta struct {
	summary     string
	description string
	examples    []string
}

func commandMetadata(dir string) (commandMeta, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return commandMeta{}, err
	}

	pkg, ok := pkgs["main"]
	if !ok {
		return commandMeta{}, fmt.Errorf("main package not found")
	}

	var files []*ast.File
	for _, file := range pkg.Files {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return fset.Position(files[i].Package).Filename < fset.Position(files[j].Package).Filename
	})

	meta := commandMeta{}
	for _, file := range files {
		if file.Doc == nil {
		} else if meta.summary == "" {
			summary := normalizeSummary(strings.TrimSpace(firstParagraph(file.Doc.Text())))
			if summary != "" {
				meta.summary = summary
			}
		}

		if err := collectFileMetadata(file, &meta); err != nil {
			return commandMeta{}, err
		}
	}

	if meta.summary == "" {
		return commandMeta{}, fmt.Errorf("package doc comment not found")
	}
	if meta.description == "" {
		return commandMeta{}, fmt.Errorf("commandPurpose constant not found")
	}
	if len(meta.examples) == 0 {
		return commandMeta{}, fmt.Errorf("commandExamples variable not found")
	}

	return meta, nil
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

func collectFileMetadata(file *ast.File, meta *commandMeta) error {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch gen.Tok {
		case token.CONST:
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if name.Name != "commandPurpose" || meta.description != "" {
						continue
					}
					if i >= len(valueSpec.Values) {
						return fmt.Errorf("commandPurpose has no value")
					}
					value, err := stringLiteral(valueSpec.Values[i])
					if err != nil {
						return err
					}
					meta.description = value
				}
			}
		case token.VAR:
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if name.Name != "commandExamples" || len(meta.examples) > 0 {
						continue
					}
					if i >= len(valueSpec.Values) {
						return fmt.Errorf("commandExamples has no value")
					}
					values, err := stringSliceLiteral(valueSpec.Values[i])
					if err != nil {
						return err
					}
					meta.examples = values
				}
			}
		}
	}

	return nil
}

func stringLiteral(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("expected string literal")
	}
	return strconv.Unquote(lit.Value)
}

func stringSliceLiteral(expr ast.Expr) ([]string, error) {
	composite, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("expected string slice literal")
	}

	values := make([]string, 0, len(composite.Elts))
	for _, elt := range composite.Elts {
		value, err := stringLiteral(elt)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func fail(err error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "generate-utils-manifest: %v\n", err)
	_, _ = os.Stderr.Write(buf.Bytes())
	os.Exit(1)
}
