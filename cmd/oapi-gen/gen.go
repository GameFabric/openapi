package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html/template"
	"io/fs"
	"slices"
	"sort"
	"strings"

	"github.com/fatih/structtag"
)

const (
	directiveGen      = "gen"
	directiveReadonly = "readonly"
	directiveRequired = "required"
	directiveFormat   = "format"
)

// Generator is a struct documentation generator. It gathers struct
// field doc blocks, making a `Docs` function that returns them as
// a map[string]string, using the field name or tag name as the key.
type Generator struct {
	tag string
	all bool
}

// NewGenerator returns a documentation generator.
func NewGenerator(tag string, all bool) *Generator {
	return &Generator{
		tag: tag,
		all: all,
	}
}

// Generate generates the documents for the given path, writing the formatted
// documentation functions to w.
func (g *Generator) Generate(path string) ([]byte, error) {
	info, err := g.gatherInfo(path)
	if err != nil {
		return nil, err
	}
	if len(info.Structs) == 0 {
		return nil, nil
	}

	tmpl, err := template.New("info").Parse(genTemplate)
	if err != nil {
		return nil, fmt.Errorf("createing template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, info); err != nil {
		return nil, fmt.Errorf("creating docs: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting docs: %w", err)
	}
	return formatted, nil
}

type pkgInfo struct {
	Pkg     string
	Structs []structInfo
}

type structInfo struct {
	Name    string
	Docs    map[string]string
	Attrs   map[string]string
	Formats map[string]string
}

func (i structInfo) Empty() bool {
	return len(i.Docs) == 0 && len(i.Attrs) == 0 && len(i.Formats) == 0
}

//nolint:cyclop,gocognit // Splitting this will not make it simpler.
func (g *Generator) gatherInfo(path string) (pkgInfo, error) {
	fset := token.NewFileSet()
	d, err := parser.ParseDir(fset, path, func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return pkgInfo{}, fmt.Errorf("parsing source code: %w", err)
	}

	switch len(d) {
	case 0:
		return pkgInfo{}, errors.New("no package found")
	case 1:
	default:
		return pkgInfo{}, errors.New("more than one package found")
	}

	var pkgName string
	for name := range d {
		pkgName = name
		break
	}

	pkg := pkgInfo{Pkg: pkgName}
	for _, f := range d[pkgName].Files {
		for _, node := range f.Decls {
			if _, ok := node.(*ast.GenDecl); !ok {
				continue
			}

			genDecl := node.(*ast.GenDecl)
			for _, spec := range genDecl.Specs {
				if _, ok := spec.(*ast.TypeSpec); !ok {
					continue
				}

				typSpec := spec.(*ast.TypeSpec)
				if _, ok := typSpec.Type.(*ast.StructType); !ok {
					continue
				}

				if !g.all && !hasDirective(directiveGen, typSpec.Doc, genDecl.Doc) {
					continue
				}

				info, err := g.gatherStructInfo(typSpec.Name.String(), typSpec.Type.(*ast.StructType))
				if err != nil {
					return pkgInfo{}, err
				}
				if info.Empty() {
					continue
				}
				pkg.Structs = append(pkg.Structs, info)
			}
		}
	}

	sort.Slice(pkg.Structs, func(i, j int) bool {
		return pkg.Structs[i].Name < pkg.Structs[j].Name
	})

	return pkg, nil
}

func (g *Generator) gatherStructInfo(name string, typ *ast.StructType) (structInfo, error) {
	info := structInfo{
		Name:    name,
		Docs:    map[string]string{},
		Attrs:   map[string]string{},
		Formats: map[string]string{},
	}
	for _, field := range typ.Fields.List {
		if field.Doc == nil {
			continue
		}

		fldName := fieldName(field, g.tag)
		if fldName == "" {
			continue
		}

		if docs := docToString(field.Doc); docs != "" {
			info.Docs[fldName] = docs
		}

		ds := directives(field.Doc)
		for _, d := range ds {
			switch {
			case d == directiveReadonly:
				info.Attrs[fldName] = "readonly"
			case d == directiveRequired && info.Attrs[fldName] == "":
				info.Attrs[fldName] = "required"
			case strings.HasPrefix(d, directiveFormat):
				_, val, found := strings.Cut(d, "=")
				if !found {
					return info, fmt.Errorf("format directive should be in format openapi:format=<format>, got %s", d)
				}
				info.Formats[fldName] = val
			}
		}
	}
	return info, nil
}

func directives(cgs ...*ast.CommentGroup) []string {
	const prefix = "//openapi:"

	if len(cgs) == 0 {
		return nil
	}

	var found []string
	for _, cg := range cgs {
		if cg == nil {
			continue
		}

		for _, comment := range cg.List {
			if strings.HasPrefix(comment.Text, prefix) {
				s := strings.TrimPrefix(comment.Text, prefix)

				// A directive should not contain spaces, ignore everything after the space.
				s, _, _ = strings.Cut(s, " ")
				if s == "" {
					continue
				}

				found = append(found, s)
			}
		}
	}
	return found
}

func hasDirective(directive string, cgs ...*ast.CommentGroup) bool {
	return slices.Contains(directives(cgs...), directive)
}

func fieldName(field *ast.Field, tag string) string {
	var fldName string
	if len(field.Names) > 0 {
		fldName = field.Names[0].String()
	}
	if field.Tag != nil {
		tags, err := structtag.Parse(strings.Trim(field.Tag.Value, "`"))
		if err != nil {
			return ""
		}
		if tag, _ := tags.Get(tag); tag != nil && tag.Name != "" {
			fldName = tag.Name
		}
	}
	return fldName
}

func docToString(cg *ast.CommentGroup) string {
	s := cg.Text()
	if s == "" {
		return ""
	}

	s = strings.Join(strings.Split(s, "\n"), " ")
	return strings.TrimSpace(s)
}

const genTemplate = `// Code generated by oapi-gen. DO NOT EDIT.

package {{ .Pkg }}

{{ range .Structs }}
{{- if .Docs }}
// Docs returns a set of property descriptions per property.
func ({{ .Name }}) Docs() map[string]string {
  return map[string]string {
  {{- range $k, $v := .Docs }}
    "{{ $k }}": "{{ $v }}",
  {{- end }}
  }
}
{{ end }}
{{- if .Attrs }}
// Attributes returns a set of property attributes per property.
func ({{ .Name }}) Attributes() map[string]string {
  return map[string]string {
  {{- range $k, $v := .Attrs }}
    "{{ $k }}": "{{ $v }}",
  {{- end }}
  }
}
{{ end }}
{{- if .Formats }}
// Formats returns a set of property formats per property.
func ({{ .Name }}) Formats() map[string]string {
  return map[string]string {
  {{- range $k, $v := .Formats }}
    "{{ $k }}": "{{ $v }}",
  {{- end }}
  }
}
{{ end }}
{{ end }}
`
