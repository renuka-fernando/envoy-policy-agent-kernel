package policy

import "net/url"

// ==================== Request Actions ====================

// ImmediateResponse returns an immediate response to the client, bypassing upstream
// Used for: authentication failures, rate limiting, validation errors, etc.
type ImmediateResponse struct {
	StatusCode int32             // HTTP status code (e.g., 401, 403, 429)
	Body       []byte            // Response body
	Headers    map[string]string // Response headers
	Reason     string            // Reason for denial (for logging/metrics)
}

func (ImmediateResponse) isRequestAction() {}

// Common Reason values for ImmediateResponse
const (
	ReasonAuthenticationFailed = "authentication_failed"
	ReasonUnauthorized         = "unauthorized"
	ReasonForbidden            = "forbidden"
	ReasonRateLimited          = "rate_limited"
	ReasonInvalidRequest       = "invalid_request"
	ReasonPolicyDenied         = "policy_denied"
)

// UpstreamResponse proceeds to upstream with optional request modifications
// Used for: modifying headers, body, path, query params before sending to upstream
type UpstreamResponse struct {
	// Header modifications
	SetHeaders    map[string]string // Headers to set or replace
	RemoveHeaders []string          // Headers to remove

	// Body modification
	Body        []byte // New request body (nil = no change)
	ContentType string // Content-Type header (empty = no change)

	// URL modification
	URL *url.URL // New URL (nil = no change)

	// HTTP method modification
	Method string // New HTTP method (empty = no change)
}

func (UpstreamResponse) isRequestAction() {}

// ==================== Response Actions ====================

// DownstreamResponse sends a response downstream to the client with optional modifications
type DownstreamResponse struct {
	// Header modifications
	SetHeaders    map[string]string // Headers to set or replace
	RemoveHeaders []string          // Headers to remove

	// Body modification
	Body        []byte // New response body (nil = no change)
	ContentType string // Content-Type header (empty = no change)

	// Status code modification
	StatusCode int32 // New status code (0 = no change)
}

func (DownstreamResponse) isResponseAction() {}
