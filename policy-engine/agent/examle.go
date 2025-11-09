package agent

import (
	"policy-engine/agent/policies"
	policy "policy-engine/policy"
)

func ListPolicies() []policy.Policy {
	return []policy.Policy{
		&policies.JWTValidationPolicy{},
	}
}
