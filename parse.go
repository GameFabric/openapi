package openapi

import (
	"reflect"
	"strings"
)

// ParseParams parses parameters from a struct using the given
// tag to derive its name.
func ParseParams(obj any, tag string) []Parameter {
	if tag == "" {
		tag = "json"
	}

	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	docable, isDocable := obj.(interface {
		Docs() map[string]string
	})

	var params []Parameter
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		switch f.Type.Kind() {
		case reflect.Interface, reflect.Struct, reflect.Pointer:
			continue
		}

		tagStr := f.Tag.Get(tag)
		if tagStr == "" || tagStr == "-" {
			continue
		}
		tagName := strings.SplitN(tagStr, ",", 2)[0]
		if tagName == "" {
			continue
		}

		var desc string
		if isDocable {
			desc = docable.Docs()[tagName]
		}

		params = append(params, QueryParameterWithType(tagName, desc, typeToJSON(f.Type.String())))
	}
	return params
}

func typeToJSON(typ string) string {
	switch typ {
	case "bool", "*bool":
		return "boolean"
	case "uint8", "*uint8", "int", "*int", "int32", "*int32", "int64", "*int64", "uint32", "*uint32", "uint64", "*uint64":
		return "integer"
	case "float64", "*float64", "float32", "*float32":
		return "number"
	case "byte", "*byte":
		fallthrough
	case "map[string]string", "*map[string]string":
		return "string"
	}
	return typ
}
