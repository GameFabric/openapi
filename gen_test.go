package openapi_test

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/gamefabric/openapi"
	kin "github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			Param(openapi.DeprecatedQueryParameter("deprecated", "the deprecated param", 123)).
			Param(openapi.HeaderParameter("Authorization", "the header authorization param")).
			Consumes("text/html", "text/plain").
			Reads(&TestObject{}).
			Produces("application/json", "application/xml").
			Returns(http.StatusOK, "OK", &TestObject{}, openapi.WithResponseHeader("X-Request-Id")).
			Returns(http.StatusNotFound, "Missing", &TestGenericObject[TestSimpleObject]{}).
			Returns(http.StatusConflict, "Conflict", "", openapi.WithMediaTypes("application/octet-steam"))

		r.With(op.Build()).Post("/test/{name}", func(rw http.ResponseWriter, req *http.Request) {})
	})

	mux.Route("/old/api", func(r chi.Router) {
		op := openapi.Op().
			ID("old-test-id").
			Doc("old test").
			Tag("old-test-tag").
			Deprecated().
			Param(openapi.PathParameter("name", "the item name")).
			Param(openapi.QueryParameter("filter", "the filter number", 123)).
			Param(openapi.QueryParameterWithType("custom", "the customer param", "integer")).
			Param(openapi.DeprecatedQueryParameter("deprecated", "the deprecated param", 123)).
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
	Test1  string `json:"test1"`
	Ignore string `json:"-"`
}

type TestObject struct {
	Test1 string   `json:"test1"`
	Test2 string   `json:"test2"`
	Test3 string   `json:"test3"`
	Test4 string   `json:"test4,omitempty"`
	Test5 []string `json:"test5,omitempty"`
}

func (TestObject) Docs() map[string]string {
	return map[string]string{
		"test1": "Test1 is an documented field with &#34;quotes&#34;\nand a newline.",
	}
}

func (TestObject) Attributes() map[string]string {
	return map[string]string{
		"test2": "readonly",
		"test3": "required",
	}
}

func (TestObject) Example() any {
	return TestObject{
		Test1: "a",
		Test2: "b",
		Test3: "c",
	}
}

func (TestObject) Formats() map[string]string {
	return map[string]string{
		"test4": "ipv4",
	}
}

func (TestObject) Enums() map[string][]string {
	return map[string][]string{
		"test4": {"192.168.1.0", "192.168.1.1"},
		"test5": {"alpha", "beta"},
	}
}

// TestPlatformName is a scalar type that implements the enumable interface.
type TestPlatformName string

func (TestPlatformName) OpenAPISchemaEnum() []string {
	return []string{"android", "ios", "pc", "playstation", "ps4", "switch", "xbox"}
}

type TestPlatformConfig struct {
	Platform TestPlatformName `json:"platform"`
}

func TestBuildSpecDynamicEnum(t *testing.T) {
	mux := chi.NewMux()

	mux.Use(openapi.Op().
		Consumes("application/json").
		Produces("application/json").
		Build())

	mux.Route("/api", func(r chi.Router) {
		op := openapi.Op().
			ID("test-dynamic-enum").
			Doc("test dynamic enum").
			Reads(&TestPlatformConfig{}).
			Produces("application/json").
			Returns(http.StatusOK, "OK", TestPlatformConfig{})

		r.With(op.Build()).Post("/config", func(rw http.ResponseWriter, req *http.Request) {})
	})

	doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
		ObjPkgSegments: 1,
	})
	require.NoError(t, err)

	// Walk to the TestPlatformConfig schema in the components.
	// The schema name includes the package prefix.
	configSchema := doc.Components.Schemas["openapi_test.TestPlatformConfig"]
	require.NotNil(t, configSchema)
	require.NotNil(t, configSchema.Value)

	platformProp := configSchema.Value.Properties["platform"]
	require.NotNil(t, platformProp)
	require.NotNil(t, platformProp.Value)

	// Verify the enum values are present.
	require.Len(t, platformProp.Value.Enum, 7)
	assert.Contains(t, platformProp.Value.Enum, "android")
	assert.Contains(t, platformProp.Value.Enum, "ios")
	assert.Contains(t, platformProp.Value.Enum, "pc")
	assert.Contains(t, platformProp.Value.Enum, "playstation")
	assert.Contains(t, platformProp.Value.Enum, "ps4")
	assert.Contains(t, platformProp.Value.Enum, "switch")
	assert.Contains(t, platformProp.Value.Enum, "xbox")
}

func TestDescribeField(t *testing.T) {
	mux := chi.NewMux()

	mux.Use(openapi.Op().Build())

	mux.Route("/api", func(r chi.Router) {
		op := openapi.Op().
			ID("create-multiple-ips").
			Doc("Creates multiple IPs").
			Describe("Does not support partial success, if there is a single error none are created.")

		r.With(op.Build()).Post("/ips", func(rw http.ResponseWriter, req *http.Request) {})
	})

	doc, err := openapi.BuildSpec(mux, openapi.SpecConfig{
		ObjPkgSegments: 1,
	})
	require.NoError(t, err)

	ops := doc.Paths.Value("/api/ips").Operations()
	require.Len(t, ops, 1)
	require.Contains(t, ops, "POST")

	opValue := ops["POST"]
	require.NotNil(t, opValue)
	assert.Equal(t, "create-multiple-ips", opValue.OperationID)
	assert.Equal(t, "Creates multiple IPs", opValue.Summary)
	assert.Equal(t, "Does not support partial success, if there is a single error none are created.", opValue.Description)
}
