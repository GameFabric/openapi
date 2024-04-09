package openapi

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	kin "github.com/getkin/kin-openapi/openapi3"
	kingen "github.com/getkin/kin-openapi/openapi3gen"
	"github.com/go-chi/chi/v5"
)

// SpecConfig configures how the spec is built.
type SpecConfig struct {
	// StripPrefixes strips the given prefixes from the routes.
	StripPrefixes []string

	// ObjPkgSegments determines the maximum number of
	// package segments to use to identify an object.
	ObjPkgSegments int
}

// BuildSpec builds openapi v3 spec from the given chi router.
func BuildSpec(r chi.Routes, cfg SpecConfig) (kin.T, error) {
	gen := newGenerator()
	gen.objPkgSegments = cfg.ObjPkgSegments

	err := chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		for _, prefix := range cfg.StripPrefixes {
			if !strings.HasPrefix(route, prefix) {
				continue
			}
			route = strings.TrimPrefix(route, prefix)
		}

		var (
			op    Operation
			found bool
		)
		for _, m := range middlewares {
			h := m(opHandler{})
			if oph, ok := h.(opHandler); ok {
				op = op.Merge(oph.Op)
				found = true
			}
		}

		if o, ok := opReg.Op(handler); ok {
			op = op.Merge(o)
			found = true
		}

		if !found || op.id == "" {
			return nil
		}

		return gen.AddOperation(method, route, op)
	})
	if err != nil {
		return kin.T{}, err
	}
	return gen.doc, nil
}

type generator struct {
	doc kin.T
	gen *kingen.Generator

	objPkgSegments int
}

func newGenerator() *generator {
	comp := kin.NewComponents()
	comp.Schemas = kin.Schemas{}
	comp.SecuritySchemes = kin.SecuritySchemes{}

	return &generator{
		doc: kin.T{
			OpenAPI:    "3.0.0",
			Components: &comp,
		},
		gen: kingen.NewGenerator(kingen.SchemaCustomizer(customizer)),
	}
}

func (g *generator) AddOperation(method, path string, op Operation) error {
	params, err := g.toParams(op.params)
	if err != nil {
		return fmt.Errorf("generating parameters for %s %q: %w", method, path, err)
	}

	reqBody, err := g.toRequestBody(op.reads, op.consumes)
	if err != nil {
		return fmt.Errorf("generating request body for %s %q: %w", method, path, err)
	}

	responses, err := g.toResponses(op.returns, op.produces)
	if err != nil {
		return fmt.Errorf("generating responses for %s %q: %w", method, path, err)
	}

	secReqs, err := g.addSecuritySchemes(op.security)
	if err != nil {
		return fmt.Errorf("generating security requirement for %s %q: %w", method, path, err)
	}

	g.doc.AddOperation(path, method, &kin.Operation{
		Summary:     op.doc,
		OperationID: op.id,
		Tags:        op.tags,
		Parameters:  params,
		RequestBody: reqBody,
		Responses:   responses,
		Security:    secReqs,
	})
	return nil
}

func (g *generator) schema(obj any) (*kin.SchemaRef, error) {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return g.gen.NewSchemaRefForValue(obj, g.doc.Components.Schemas)
	}

	name := t.Name()
	if path := t.PkgPath(); path != "" && g.objPkgSegments > 0 {
		parts := strings.Split(path, "/")
		if l := len(parts); l > g.objPkgSegments {
			parts = parts[l-g.objPkgSegments:]
		}
		name = strings.Join(parts, ".") + "." + name
	}

	if _, ok := g.doc.Components.Schemas[name]; ok {
		return &kin.SchemaRef{Ref: "#/components/schemas/" + name}, nil
	}

	schema, err := g.gen.NewSchemaRefForValue(obj, g.doc.Components.Schemas)
	if err != nil {
		return nil, err
	}
	if schema.Value == nil || !isExported(t.Name()) {
		return schema, nil
	}

	g.doc.Components.Schemas[name] = schema

	return &kin.SchemaRef{Ref: "#/components/schemas/" + name}, nil
}

func isExported(name string) bool {
	runes := []rune(name)
	if len(runes) == 0 {
		return false
	}
	return !unicode.IsLower(runes[0])
}

func (g *generator) toParams(params []Parameter) (kin.Parameters, error) {
	if len(params) == 0 {
		return nil, nil
	}

	ret := make([]*kin.ParameterRef, len(params))
	for i, param := range params {
		var (
			schema *kin.SchemaRef
			err    error
		)
		switch {
		case param.typ != "":
			schema = &kin.SchemaRef{
				Value: &kin.Schema{
					Type: &kin.Types{param.typ},
				},
			}
		case param.dataType != nil:
			schema, err = g.schema(param.dataType)
			if err != nil {
				return nil, err
			}
		}

		ret[i] = &kin.ParameterRef{Value: &kin.Parameter{
			Name:        param.name,
			In:          param.in,
			Description: param.description,
			Required:    param.required,
			Schema:      schema,
		}}
	}
	return ret, nil
}

func (g *generator) toRequestBody(obj any, mediaTypes []string) (*kin.RequestBodyRef, error) {
	if obj == nil || len(mediaTypes) == 0 {
		//nolint:nilnil
		return nil, nil
	}

	schema, err := g.schema(obj)
	if err != nil {
		return nil, err
	}

	content := kin.Content{}
	for _, mime := range mediaTypes {
		content[mime] = &kin.MediaType{Schema: schema}
	}

	return &kin.RequestBodyRef{Value: &kin.RequestBody{
		Required: true,
		Content:  content,
	}}, nil
}

func (g *generator) toResponses(res []Response, mediaTypes []string) (*kin.Responses, error) {
	if len(res) == 0 || len(mediaTypes) == 0 {
		return nil, nil
	}

	responses := &kin.Responses{}
	for _, r := range res {
		if r.writes == nil {
			responses.Set(strconv.Itoa(r.code), &kin.ResponseRef{Value: &kin.Response{
				Description: &r.description,
			}})
			continue
		}

		schema, err := g.schema(r.writes)
		if err != nil {
			return nil, err
		}

		content := kin.Content{}
		for _, mime := range mediaTypes {
			content[mime] = &kin.MediaType{Schema: schema}
		}

		headers := make(kin.Headers, len(r.headers))
		for _, name := range r.headers {
			headers[name] = &kin.HeaderRef{
				Value: &kin.Header{
					Parameter: kin.Parameter{
						Name: name,
						In:   kin.ParameterInHeader,
					},
				},
			}
		}

		responses.Set(strconv.Itoa(r.code), &kin.ResponseRef{Value: &kin.Response{
			Description: &r.description,
			Content:     content,
			Headers:     headers,
		}})
	}
	return responses, nil
}

// addSecuritySchemes derives a security scheme from the given Security struct and returns the security requirements,
// which act as a reference from an endpoint to its security scheme.
func (g *generator) addSecuritySchemes(secs map[string]Security) (*kin.SecurityRequirements, error) {
	if len(secs) == 0 {
		return nil, nil //nolint:nilnil
	}

	var reqs kin.SecurityRequirements
	for name, sec := range secs {
		switch sec.Type {
		case secTypeBasic:
			g.doc.Components.SecuritySchemes[name] = &kin.SecuritySchemeRef{
				Value: &kin.SecurityScheme{
					Type:   "http",
					Scheme: "basic",
				},
			}
			reqs = append(reqs, kin.SecurityRequirement{
				name: []string{},
			})
		case secTypeBearer:
			g.doc.Components.SecuritySchemes[name] = &kin.SecuritySchemeRef{
				Value: &kin.SecurityScheme{
					BearerFormat: sec.BearerFormat,
					Type:         "http",
					Scheme:       "bearer",
				},
			}
			reqs = append(reqs, kin.SecurityRequirement{
				name: []string{},
			})
		case secTypeAPIKey:
			g.doc.Components.SecuritySchemes[name] = &kin.SecuritySchemeRef{
				Value: &kin.SecurityScheme{
					Type: "apiKey",
					Name: sec.APIKeyName,
					In:   sec.APIKeyIn,
				},
			}
			reqs = append(reqs, kin.SecurityRequirement{
				name: []string{},
			})
		default:
			return nil, fmt.Errorf("unsupported security type %q", sec.Type)
		}
	}

	return &reqs, nil
}

type openAPIType interface {
	OpenAPISchemaType() []string
	OpenAPISchemaFormat() string
}

type oneOfTypes interface {
	OpenAPIV3OneOfTypes() []string
}

type docable interface {
	Docs() map[string]string
}

type attrable interface {
	Attributes() map[string]string
}

type formatable interface {
	Formats() map[string]string
}

func customizer(name string, t reflect.Type, _ reflect.StructTag, schema *kin.Schema) error {
	v := reflect.New(t).Elem().Interface()

	if obj, ok := v.(openAPIType); ok {
		if err := applyType(name, schema, obj); err != nil {
			return err
		}
	}

	if obj, ok := v.(oneOfTypes); ok {
		applyOneOfTypes(schema, obj)
	}

	if obj, ok := v.(docable); ok {
		applyDocs(schema, obj)
	}

	if obj, ok := v.(attrable); ok {
		applyAttrs(schema, obj)
	}

	if obj, ok := v.(formatable); ok {
		applyFormats(schema, obj)
	}

	return nil
}

func applyType(name string, schema *kin.Schema, obj openAPIType) error {
	typs := obj.OpenAPISchemaType()
	if len(typs) == 0 {
		return fmt.Errorf("type %q defines open api types by returns none", name)
	}
	schema.Type = &kin.Types{typs[0]}
	schema.Format = obj.OpenAPISchemaFormat()
	return nil
}

func applyOneOfTypes(schema *kin.Schema, obj oneOfTypes) {
	typs := obj.OpenAPIV3OneOfTypes()
	var refs kin.SchemaRefs
	for _, typ := range typs {
		refs = append(refs, &kin.SchemaRef{Value: &kin.Schema{Type: &kin.Types{typ}}})
	}
	schema.OneOf = refs
}

func applyDocs(schema *kin.Schema, obj docable) {
	docs := obj.Docs()
	for k, prop := range schema.Properties {
		doc, ok := docs[k]
		if !ok {
			continue
		}
		if prop.Value == nil {
			continue
		}

		prop.Value.Description = doc
	}
}

func applyAttrs(schema *kin.Schema, obj attrable) {
	attrs := obj.Attributes()
	var required []string
	for k, prop := range schema.Properties {
		attr := attrs[k]
		if prop.Value == nil {
			continue
		}

		switch attr {
		case "readonly":
			prop.Value.ReadOnly = true
		case "required":
			required = append(required, k)
		}
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema.Required = required
	}
}

func applyFormats(schema *kin.Schema, obj formatable) {
	fmts := obj.Formats()
	for k, prop := range schema.Properties {
		fmt := fmts[k]
		if prop.Value == nil {
			continue
		}

		prop.Value.WithFormat(fmt)
	}
}
