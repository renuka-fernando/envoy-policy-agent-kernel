package policies

import (
	"context"
	policy "policy-engine/policy"
)

// Example 1: Request-only policy (JWT validation)
type JWTValidationPolicy struct {
}

func (p *JWTValidationPolicy) Name() string                        { return "jwtValidation" }
func (p *JWTValidationPolicy) Version() string                     { return "1.0.0" }
func (p *JWTValidationPolicy) SupportedPhases() policy.PolicyPhase { return policy.PhaseRequest }
func (p *JWTValidationPolicy) ParameterSchema() []string {
	return []string{"issuer", "audience"}
}
func (p *JWTValidationPolicy) AccessRequestBody() bool  { return false }
func (p *JWTValidationPolicy) AccessResponseBody() bool { return false }

func (p *JWTValidationPolicy) HandleRequest(ctx context.Context, req *policy.RequestContext) *policy.RequestResult {
	authHeader := req.Request.Headers["Authorization"]
	if len(authHeader) == 0 {
		// Business logic denial
		return &policy.RequestResult{
			Action: policy.ImmediateResponse{
				StatusCode: 401,
				Body:       []byte(`{"error":"Missing authorization header"}`),
				Headers:    map[string]string{"Content-Type": "application/json"},
				Reason:     policy.ReasonAuthenticationFailed,
			},
			Message: "Authorization header missing",
		}
	}

	// Success - proceed to upstream with added header
	return &policy.RequestResult{
		Action: policy.UpstreamResponse{
			SetHeaders: map[string]string{
				"X-User-ID": "renuka",
			},
		},
		Message: "JWT validation successful",
		Metadata: map[string]string{
			"authenticated_user_id": "renuka",
			"user_tier":             "silver",
		},
	}
}

func (p *JWTValidationPolicy) HandleResponse(ctx context.Context, req *policy.ResponseContext) *policy.ResponseResult {
	return nil // Not used for request-only policy
}
