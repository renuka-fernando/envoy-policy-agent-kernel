package agent

import (
	"policy-engine/agent/policies"
	policy "policy-engine/policy"
)

// ListPolicies returns the request and response policies separately.
// Request policies are executed during the request phase, and response policies
// during the response phase. Policies that support both phases should be included
// in both lists.
func ListPolicies() (requestPolicies []policy.Policy, responsePolicies []policy.Policy) {
	requestPolicies = []policy.Policy{
		&policies.JWTValidationPolicy{},
	}

	responsePolicies = []policy.Policy{
		// Add response-only policies here
	}

	return requestPolicies, responsePolicies
}
