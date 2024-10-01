package openapi_test

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gamefabric/openapi"
	kin "github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func Example() {
	mux := chi.NewMux()

	// `openapi` will build the route additively.
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
			Consumes("text/html", "text/plain").
			Reads(&TestObject{}).
			Produces("application/json", "application/xml").
			Returns(http.StatusOK, "OK", &TestObject{})

		r.With(op.Build()).Post("/test/{name}", func(rw http.ResponseWriter, req *http.Request) {})
	})

	mux.Get("/internal/handler", testHandler())

	doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
		StripPrefixes:  []string{"/internal"},
		ObjPkgSegments: 1,
	})
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	doc.OpenAPI = "3.0.0"
	doc.Info = &kin.Info{
		Title:   "Test Server",
		Version: "1",
	}
	_, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
}
