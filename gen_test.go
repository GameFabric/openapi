package openapi_test

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strconv"
	"testing"

	kin "github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/nitrado/b2b/go/openapi"
)

var update = flag.Bool("update", false, "update the golden files of this test")

func TestBuildSpec(t *testing.T) {
	mux := chi.NewMux()

	mux.Use(openapi.Op().
		Consumes("application/json").
		Produces("application/json").
		Build())

	mux.Route("/api", func(r chi.Router) {
		op := openapi.Op().
			ID("test-id").
			Doc("test").
			Tag("test-tag").
			Param(openapi.PathParameter("name", "the item name")).
			Param(openapi.QueryParameter("filter", "the filter number", 123)).
			Param(openapi.QueryParameterWithType("custom", "the customer param", "integer")).
			Param(openapi.HeaderParameter("Authorization", "the header authorization param")).
			Consumes("text/html", "text/plain").
			Reads(&TestObject{}).
			Produces("application/json", "application/xml").
			Returns(http.StatusOK, "OK", &TestObject{}, openapi.WithResponseHeader("X-Request-Id")).
			Returns(http.StatusNotFound, "Missing", &TestGenericObject[TestSimpleObject]{}).
			Returns(http.StatusConflict, "Conflict", "", openapi.WithMediaTypes("application/octet-steam"))

		r.With(op.Build()).Post("/test/{name}", func(rw http.ResponseWriter, req *http.Request) {})
	})

	mux.Get("/internal/handler", testHandler())

	for i := range 2 {
		t.Run("pkg segments "+strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
				StripPrefixes:  []string{"/internal"},
				ObjPkgSegments: i,
			})
			require.NoError(t, err)

			doc.OpenAPI = "3.0.0"
			doc.Info = &kin.Info{
				Title:   "Test Server",
				Version: "1",
			}
			got, err := json.MarshalIndent(doc, "", "  ")
			require.NoError(t, err)

			name := "testdata/spec.json"
			if i != 1 {
				name = "testdata/spec-pkgseg" + strconv.Itoa(i) + ".json"
			}

			if *update {
				_ = os.WriteFile(name, got, 0o644)
			}

			want, err := os.ReadFile(name)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestBuildSpecSecurity(t *testing.T) {
	mux := chi.NewMux()

	mux.Use(openapi.Op().Build())

	mux.Route("/api", func(r chi.Router) {
		testOp := func(id string) *openapi.OpBuilder {
			return openapi.Op().
				ID(id).
				Produces("application/json", "application/xml").
				Returns(http.StatusNoContent, http.StatusText(http.StatusNoContent), nil)
		}

		// Basic:
		r.With(testOp("test-basic1").RequiresAuth("myBasicAuth", openapi.SecurityBasic).Build()).Post("/basic", func(rw http.ResponseWriter, req *http.Request) {})
		r.With(testOp("test-basic2").RequiresAuth("myBasicAuth", openapi.SecurityBasic).Build()).Post("/basic-reuse", func(rw http.ResponseWriter, req *http.Request) {})

		// Bearer:
		r.With(testOp("test-bearer").RequiresAuth("myBearerAuth", openapi.SecurityBearer).Build()).Post("/bearer", func(rw http.ResponseWriter, req *http.Request) {})
		r.With(testOp("test-jwt").RequiresAuth("myJWTAuth", openapi.Security{
			Type:         "bearer",
			BearerFormat: "JWT",
		}).Build()).Post("/bearer-jwt", func(rw http.ResponseWriter, req *http.Request) {})

		// APIKey:
		r.With(testOp("test-apikey-header").RequiresAuth("myHeaderAPIKey", openapi.Security{
			Type:       "apiKey",
			APIKeyName: "Foo",
			APIKeyIn:   "header",
		}).Build()).Post("/apikey-header", func(rw http.ResponseWriter, req *http.Request) {})

		r.With(testOp("test-apikey-cookie").RequiresAuth("myCookieAPIKey", openapi.Security{
			Type:       "apiKey",
			APIKeyName: "Foo",
			APIKeyIn:   "cookie",
		}).Build()).Post("/apikey-cookie", func(rw http.ResponseWriter, req *http.Request) {})

		r.With(testOp("test-apikey-query").RequiresAuth("myQueryAPIKey", openapi.Security{
			Type:       "apiKey",
			APIKeyName: "foo",
			APIKeyIn:   "query",
		}).Build()).Post("/apikey-query", func(rw http.ResponseWriter, req *http.Request) {})
	})

	doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
		ObjPkgSegments: 1,
	})
	require.NoError(t, err)

	doc.OpenAPI = "3.0.0"
	doc.Info = &kin.Info{
		Title:   "Test Server",
		Version: "1",
	}
	got, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	if *update {
		_ = os.WriteFile("testdata/spec-security.json", got, 0o644)
	}

	want, err := os.ReadFile("testdata/spec-security.json")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func testHandler() http.HandlerFunc {
	type options struct {
		PageSize int    `schema:"page_size"`
		Token    string `schema:"token"`
	}
	type obj struct {
		Test int `json:"test"`
	}

	docs := openapi.Op().
		ID("test-handler").
		Doc("test handler").
		Tag("handler").
		Params(openapi.ParseParams(options{}, "schema")...).
		Returns(http.StatusOK, "OK", obj{}).
		Returns(http.StatusNoContent, "OK", nil).
		BuildHandler()

	return docs(func(rw http.ResponseWriter, req *http.Request) {})
}

type TestGenericObject[T TestSimpleObject] struct {
	Test1 T      `json:"test1"`
	Test2 string `json:"test2"`
}

type TestSimpleObject struct {
	Test1 string `json:"test1"`
}

type TestObject struct {
	Test1 string `json:"test1"`
	Test2 string `json:"test2"`
	Test3 string `json:"test3"`
	Test4 string `json:"test4"`
}

func (TestObject) Docs() map[string]string {
	return map[string]string{
		"test1": "Some test docs",
	}
}

func (TestObject) Attributes() map[string]string {
	return map[string]string{
		"test2": "readonly",
		"test3": "required",
	}
}

func (TestObject) Formats() map[string]string {
	return map[string]string{
		"test4": "ipv4",
	}
}
