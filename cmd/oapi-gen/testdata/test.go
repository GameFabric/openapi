package testdata

// TestObject is a test object.
//
//openapi:gen
type TestObject struct {
	// A is an example field with "quotes"
	// and a newline.
	A string `json:"a"`

	// B is another example field.
	//
	//openapi:required // This should be ignored
	B string

	// C should not appear
	C string `json:"-"`
}

type TestOtherObject struct {
	// C is an example field.
	C string `json:"c"`

	// D is another example field.
	//
	//openapi:readonly // This should be ignored
	D string

	// E is a formatted example field.
	//
	//openapi:format=ipv4 // This should be ignored
	E string
}
