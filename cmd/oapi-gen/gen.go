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
	"sort"
	"strings"

	"github.com/fatih/structtag"
)

const (
	directiveGen      = "openapi:gen"
	directiveReadonly = "openapi:readonly"
	directiveRequired = "openapi:required"
	directiveFormat   = "openapi:format"
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
	Props   map[string]string
	Formats map[string]string
}

//nolint:cyclop // Splitting this will not make it simpler.
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

				docs := g.gatherStructDocs(typSpec.Type.(*ast.StructType))
				props := g.gatherStructProps(typSpec.Type.(*ast.StructType))
				formats := g.gatherStructFormats(typSpec.Type.(*ast.StructType))

				if len(docs) == 0 && len(props) == 0 && len(formats) == 0 {
					continue
				}

				pkg.Structs = append(pkg.Structs, structInfo{
					Name:    typSpec.Name.String(),
					Docs:    docs,
					Props:   props,
					Formats: formats,
				})
			}
		}
	}

	sort.Slice(pkg.Structs, func(i, j int) bool {
		return pkg.Structs[i].Name < pkg.Structs[j].Name
	})

	return pkg, nil
}

func (g *Generator) gatherStructDocs(typ *ast.StructType) map[string]string {
	docs := map[string]string{}
	for _, field := range typ.Fields.List {
		fldName := fieldName(field, g.tag)
		if fldName == "" {
			continue
		}

		var comment string
		if field.Doc != nil {
			comment = docToString(field.Doc)
		}
		if comment == "" {
			continue
		}

		docs[fldName] = comment
	}
	return docs
}

func (g *Generator) gatherStructProps(typ *ast.StructType) map[string]string {
	props := map[string]string{}
	for _, field := range typ.Fields.List {
		fldName := fieldName(field, g.tag)
		if fldName == "" {
			continue
		}

		switch {
		case hasDirective(directiveReadonly, field.Doc):
			props[fldName] = "readonly"
		case hasDirective(directiveRequired, field.Doc):
			props[fldName] = "required"
		}
	}
	return props
}

func (g *Generator) gatherStructFormats(typ *ast.StructType) map[string]string {
	formats := map[string]string{}
	for _, field := range typ.Fields.List {
		fldName := fieldName(field, g.tag)
		if fldName == "" {
			continue
		}

		if rest, found := cutDirective(directiveFormat+"=", field.Doc); found {
			formats[fldName] = rest
		}
	}
	return formats
}

func hasDirective(directive string, cgs ...*ast.CommentGroup) bool {
	_, found := cutDirective(directive, cgs...)
	return found
}

func cutDirective(directive string, cgs ...*ast.CommentGroup) (string, bool) {
	if len(cgs) == 0 {
		return "", false
	}

	for _, cg := range cgs {
		if cg == nil {
			continue
		}

		for _, comment := range cg.List {
			if _, after, found := strings.Cut(comment.Text, "//"+directive); found {
				return after, true
			}
		}
	}
	return "", false
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

const genTemplate = `package {{ .Pkg }}

// Code generated by oapi-gen. DO NOT EDIT.
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
{{- if .Props }}
// Attributes returns a set of property attributes per property.
func ({{ .Name }}) Attributes() map[string]string {
  return map[string]string {
  {{- range $k, $v := .Props }}
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
