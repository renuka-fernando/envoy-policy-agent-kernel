package agent

import (
	"policy-engine/agent/policies"
	policy "policy-engine/policy"
)

func ListPolicies() []policy.PolicyTask {
	return []policy.PolicyTask{
		policy.PolicyTask{
			Policy: &policies.JWTValidationPolicy{},
		},
	}
}
