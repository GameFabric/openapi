package openapi

import (
	"net/http"
	"reflect"
	"sync"

	kin "github.com/getkin/kin-openapi/openapi3"
)

// Parameter documents a query or path parameter.
type Parameter struct {
	in          string
	name        string
	description string
	required    bool
	typ         string
	dataType    any
}

// PathParameter returns a path parameter with the given name and description.
func PathParameter(name, description string) Parameter {
	return Parameter{
		in:          kin.ParameterInPath,
		name:        name,
		description: description,
		required:    true,
		dataType:    "",
	}
}

// QueryParameter returns a query parameter where the type will be resolved.
func QueryParameter(name, description string, typ any) Parameter {
	return Parameter{
		in:          kin.ParameterInQuery,
		name:        name,
		description: description,
		dataType:    typ,
	}
}

// QueryParameterWithType returns a query parameter with the given type.
func QueryParameterWithType(name, description, typ string) Parameter {
	return Parameter{
		in:          kin.ParameterInQuery,
		name:        name,
		description: description,
		typ:         typ,
	}
}

// Response documents a request response.
type Response struct {
	code        int
	description string
	writes      any
}

// Operation documents a request.
type Operation struct {
	id       string
	tags     []string
	doc      string
	params   []Parameter
	consumes []string
	reads    any
	produces []string
	returns  []Response
}

// Merge merges the operation with the given operation.
func (o Operation) Merge(newOp Operation) Operation {
	if newOp.id != "" {
		o.id = newOp.id
	}
	if newOp.doc != "" {
		o.doc = newOp.doc
	}
	if len(newOp.tags) > 0 {
		o.tags = append([]string{}, o.tags...)
		o.tags = append(o.tags, newOp.tags...)
	}
	if len(newOp.params) > 0 {
		o.params = append([]Parameter{}, o.params...)
		o.params = append(o.params, newOp.params...)
	}
	if len(newOp.consumes) > 0 {
		o.consumes = append([]string{}, o.consumes...)
		o.consumes = append(o.consumes, newOp.consumes...)
	}
	if newOp.reads != nil {
		o.reads = newOp.reads
	}
	if len(newOp.produces) > 0 {
		o.produces = append([]string{}, o.produces...)
		o.produces = append(o.produces, newOp.produces...)
	}
	if len(newOp.returns) > 0 {
		o.returns = append([]Response{}, o.returns...)
		o.returns = append(o.returns, newOp.returns...)
	}
	return o
}

// OpBuilder builds an operation. An operation describes a request route.
type OpBuilder struct {
	op *Operation
}

// Op returns an op builder.
func Op() *OpBuilder {
	return &OpBuilder{op: &Operation{}}
}

// ID set the operation id.
func (o *OpBuilder) ID(id string) *OpBuilder {
	o.op.id = id
	return o
}

// Doc sets the operation summary.
func (o *OpBuilder) Doc(doc string) *OpBuilder {
	o.op.doc = doc
	return o
}

// Tag appends the given tag to the operation.
func (o *OpBuilder) Tag(tag string) *OpBuilder {
	o.op.tags = append(o.op.tags, tag)
	return o
}

// Param appends the given parameter to the operation.
func (o *OpBuilder) Param(param Parameter) *OpBuilder {
	o.op.params = append(o.op.params, param)
	return o
}

// Params appends the given parameters to the operation.
func (o *OpBuilder) Params(params ...Parameter) *OpBuilder {
	o.op.params = append(o.op.params, params...)
	return o
}

// Consumes appends the given consumable media types to the operation.
func (o *OpBuilder) Consumes(mediaTypes ...string) *OpBuilder {
	o.op.consumes = mediaTypes
	return o
}

// Reads sets the request body type on the operation.
func (o *OpBuilder) Reads(obj any) *OpBuilder {
	o.op.reads = obj
	return o
}

// Produces appends the given producible media types to the operation.
func (o *OpBuilder) Produces(mediaTypes ...string) *OpBuilder {
	o.op.produces = mediaTypes
	return o
}

// Returns appends the given response to the operation.
func (o *OpBuilder) Returns(code int, description string, obj any) *OpBuilder {
	o.op.returns = append(o.op.returns, Response{
		code:        code,
		description: description,
		writes:      obj,
	})
	return o
}

// Build builds a middleware that will return an Operation when queried.
// In all other situations, the given handler is returned, effectively
// removing the middleware from the stack.
func (o *OpBuilder) Build() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if _, ok := next.(opHandler); ok {
			return opHandler{Op: *o.op}
		}
		return next
	}
}

// BuildHandler builds a wrapper handler that contains an Operation.
//
// The operation is registered globally with the operation registrar
// as there is no other way to retrieve the operation from a handler func.
func (o *OpBuilder) BuildHandler() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		opReg.Register(next, *o.op)

		return next
	}
}

type opHandler struct {
	Op Operation
}

func (o opHandler) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

var opReg = registrar{}

type registrar struct {
	mu  sync.Mutex
	ops map[reflect.Value]Operation
}

func (r *registrar) Register(i any, op Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ops == nil {
		r.ops = map[reflect.Value]Operation{}
	}

	v := reflect.ValueOf(i)
	r.ops[v] = op
}

func (r *registrar) Op(i any) (Operation, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	v := reflect.ValueOf(i)
	op, ok := r.ops[v]
	return op, ok
}
