package policy

import "net/url"

// RequestContext contains all data needed for request phase policy execution
type RequestContext struct {
	// RequestID is a unique identifier for this request

	// Metadata contains data passed from previous policies in the chain
	Metadata map[string]string

	// Request contains HTTP request data
	Request *RequestData
}

// ResponseContext contains all data needed for response phase policy execution
type ResponseContext struct {
	// RequestID is a unique identifier for this request
	RequestID string

	// Metadata contains data passed from previous policies in the chain
	Metadata map[string]string

	// Request contains the original HTTP request data
	Request *RequestData

	// Response contains HTTP response data from upstream
	Response *ResponseData
}

// RequestData contains HTTP request information
type RequestData struct {
	Headers        map[string][]string // HTTP headers (multi-value support)
	Body           []byte              // Request body (optional)
	BodyIncluded   bool                // Indicates if body is present
	Method         string              // HTTP method (GET, POST, etc.)
	Path           *url.URL            // URL path
	Scheme         string              // http or https
	Authority      string              // Host header value
	ClientIP       string              // Client IP address
	ForwardedProto string              // X-Forwarded-Proto header value
}

// ResponseData contains HTTP response information
type ResponseData struct {
	Headers      map[string][]string // HTTP response headers
	Body         []byte              // Response body (optional)
	BodyIncluded bool                // Indicates if body is present
	StatusCode   int32               // HTTP status code
}
