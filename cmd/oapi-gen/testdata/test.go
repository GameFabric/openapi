package testdata

// TestObject is a test object.
//
//openapi:gen
type TestObject struct {
	// A is an example field with "quotes".
	A string `json:"a"`

	// B is another example field.
	//
	//openapi:required // This should be ignored
	B string
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
