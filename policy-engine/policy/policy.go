package policy

import "context"

// Policy represents a single policy implementation that can be executed
// by a Policy Agent. Each Policy is responsible for:
// - Declaring its metadata (name, version, supported phases, parameters)
// - Executing policy logic for request and/or response phases
type Policy interface {
	// Name returns the unique policy identifier (e.g., "jwtValidation", "rateLimit")
	Name() string

	// Version returns the policy implementation version (e.g., "1.0.0")
	Version() string

	// SupportedPhases returns which processing phases this policy can handle
	SupportedPhases() PolicyPhase

	// ParameterSchema returns the list of parameter names this policy accepts
	// Used by the kernel for validation during startup configuration checks
	ParameterSchema() []string

	// AccessRequestBody returns true if this policy needs access to the request body
	// Used by the kernel to determine if request body buffering is required
	AccessRequestBody() bool

	// AccessResponseBody returns true if this policy needs access to the response body
	// Used by the kernel to determine if response body buffering is required
	AccessResponseBody() bool

	// HandleRequest executes policy logic during the REQUEST phase
	// Called before the request is sent to the upstream service
	// Return nil if this policy doesn't handle the request phase
	HandleRequest(ctx context.Context, req *RequestContext) *RequestResult

	// HandleResponse executes policy logic during the RESPONSE phase
	// Called after receiving the response from the upstream service
	// Return nil if this policy doesn't handle the response phase
	HandleResponse(ctx context.Context, req *ResponseContext) *ResponseResult
}

// PolicyPhase indicates when a policy can be executed
type PolicyPhase int

const (
	PhaseRequest         PolicyPhase = 0 // REQUEST phase only
	PhaseResponse        PolicyPhase = 1 // RESPONSE phase only
	PhaseRequestResponse PolicyPhase = 2 // Both REQUEST and RESPONSE phases
)

// RequestResult is returned by request phase policy execution
type RequestResult struct {
	// Action to take (ImmediateResponse or UpstreamResponse)
	Action RequestAction

	// Message provides human-readable details about the execution
	Message string

	// Metadata to pass to subsequent policies in the chain
	Metadata map[string]string
}

// ResponseResult is returned by response phase policy execution
type ResponseResult struct {
	// Action to take (DownstreamResponse)
	Action ResponseAction

	// Message provides human-readable details about the execution
	Message string

	// Metadata to pass to subsequent policies in the chain
	Metadata map[string]string
}

// RequestAction represents the action to take during request phase
// This is a sealed/closed sum type - only predefined types can implement it
// Implementations: ImmediateResponse, UpstreamResponse
type RequestAction interface {
	// isRequestAction is an unexported method that seals this interface.
	// Types outside this package cannot implement RequestAction.
	isRequestAction()
}

// ResponseAction represents the action to take during response phase
// This is a sealed/closed sum type - only predefined types can implement it
// Implementations: DownstreamResponse
type ResponseAction interface {
	// isResponseAction is an unexported method that seals this interface.
	// Types outside this package cannot implement ResponseAction.
	isResponseAction()
}

// ==================== Helper Functions ====================

// InternalError creates a RequestResult with a 500 Internal Server Error response
// Use this when policy execution encounters operational errors (e.g., database failure,
// external service unavailable, parsing errors)
func InternalError(message string) *RequestResult {
	return &RequestResult{
		Action: ImmediateResponse{
			StatusCode: 500,
			Body:       []byte(`{"error":"Internal server error","code":"POLICY_ERROR"}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
			Reason:     "policy_error",
		},
		Message: message,
	}
}

// InternalErrorWithDetails creates a RequestResult with a 500 error and custom error details
func InternalErrorWithDetails(message string, errorDetails string) *RequestResult {
	body := []byte(`{"error":"` + errorDetails + `","code":"POLICY_ERROR"}`)
	return &RequestResult{
		Action: ImmediateResponse{
			StatusCode: 500,
			Body:       body,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Reason:     "policy_error",
		},
		Message: message,
	}
}
