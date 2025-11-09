package policy

import (
	"context"
)

func AccessRequestBody(policies []Policy) bool {
	for _, p := range policies {
		if p.AccessRequestBody() {
			return true
		}
	}
	return false
}

func AccessResponseBody(policies []Policy) bool {
	for _, p := range policies {
		if p.AccessResponseBody() {
			return true
		}
	}
	return false
}

func ExecuteRequestPolicies(ctx context.Context, policies []Policy, reqCtx *RequestContext) {
	for _, p := range policies {
		result := p.HandleRequest(ctx, reqCtx)
		if result != nil {
			// Process result (e.g., update metadata, handle instructions)
			for k, v := range result.Metadata {
				reqCtx.Metadata[k] = v
			}
			// Handle instructions as needed
		}
	}
}
