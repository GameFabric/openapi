package openapi_test

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
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
			Returns(http.StatusOK, "OK", &TestObject{}, openapi.WithResponseHeader("X-Request-Id"))

		r.With(op.Build()).Post("/test/{name}", func(rw http.ResponseWriter, req *http.Request) {})
	})

	mux.Get("/internal/handler", testHandler())

	doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
		StripPrefixes:  []string{"/internal"},
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
		_ = os.WriteFile("testdata/spec.json", got, 0o644)
	}

	want, err := os.ReadFile("testdata/spec.json")
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

type TestObject struct {
	Test1 string `json:"test1"`
	Test2 string `json:"test2"`
	Test3 string `json:"test3"`
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
