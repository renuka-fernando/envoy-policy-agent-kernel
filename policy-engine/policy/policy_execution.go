package policy

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// PolicyExecution represents the execution state and metadata for a collection of policies.
// It encapsulates a set of policy tasks along with their collective execution status,
// allowing tracking of the overall policy execution lifecycle independently from
// individual policy task states.
type PolicyExecution struct {
	// Policies contains the list of policy tasks to be executed
	Policies []PolicyTask

	// Status tracks the overall execution state of all policies
	Status ExecutionStatus

	// StartedAt timestamp when policy execution began
	StartedAt time.Time

	// CompletedAt timestamp when all policies completed
	CompletedAt time.Time

	// Error tracks any fatal error that stopped the entire execution
	Error error

	// accessRequestBody indicates if any policy needs to access the request body
	accessRequestBody bool

	// accessResponseBody indicates if any policy needs to access the response body
	accessResponseBody bool
}

// ExecutionStatus represents the overall state of policy execution
type ExecutionStatus string

const (
	// ExecutionStatusPending indicates policy execution has not yet started
	ExecutionStatusPending ExecutionStatus = "pending"

	// ExecutionStatusInProgress indicates policy execution is currently running
	ExecutionStatusInProgress ExecutionStatus = "in_progress"

	// ExecutionStatusCompleted indicates all policies completed successfully
	ExecutionStatusCompleted ExecutionStatus = "completed"

	// ExecutionStatusFailed indicates execution failed with a fatal error
	ExecutionStatusFailed ExecutionStatus = "failed"

	// ExecutionStatusPartial indicates some policies completed while others failed
	ExecutionStatusPartial ExecutionStatus = "partial"
)

// NewPolicyExecution creates a new PolicyExecution with the given policies
// It wraps each policy in a PolicyTask with initial status
func NewPolicyExecution(policies []Policy) *PolicyExecution {
	policyTasks := make([]PolicyTask, len(policies))
	for i, p := range policies {
		policyTasks[i] = PolicyTask{
			Policy: p,
			Status: PolicyTaskStatus{
				IsCompleted:             false,
				LastExecutedStreamIndex: 0,
			},
		}
	}

	pe := &PolicyExecution{
		Policies: policyTasks,
		Status:   ExecutionStatusPending,
	}

	// Initialize body access flags
	pe.Init()

	return pe
}

// Init initializes the PolicyExecution by determining body access requirements
func (pe *PolicyExecution) Init() {
	pe.accessRequestBody = pe.checkAccessRequestBody()
	pe.accessResponseBody = pe.checkAccessResponseBody()
}

// checkAccessRequestBody checks if any policy needs to access the request body
func (pe *PolicyExecution) checkAccessRequestBody() bool {
	for _, p := range pe.Policies {
		if p.Policy.AccessRequestBody() {
			return true
		}
	}
	return false
}

// checkAccessResponseBody checks if any policy needs to access the response body
func (pe *PolicyExecution) checkAccessResponseBody() bool {
	for _, p := range pe.Policies {
		if p.Policy.AccessResponseBody() {
			return true
		}
	}
	return false
}

// AccessRequestBody returns whether any policy needs to access the request body
func (pe *PolicyExecution) AccessRequestBody() bool {
	return pe.accessRequestBody
}

// AccessResponseBody returns whether any policy needs to access the response body
func (pe *PolicyExecution) AccessResponseBody() bool {
	return pe.accessResponseBody
}

// AllCompleted checks if all policy tasks have completed
func (pe *PolicyExecution) AllCompleted() bool {
	for _, p := range pe.Policies {
		if !p.Status.IsCompleted {
			return false
		}
	}
	return true
}

// AnyCompleted checks if any policy task has completed
func (pe *PolicyExecution) AnyCompleted() bool {
	for _, p := range pe.Policies {
		if p.Status.IsCompleted {
			return true
		}
	}
	return false
}

// ExecuteRequestPolicies executes all policies in the execution for request processing
func (pe *PolicyExecution) ExecuteRequestPolicies(ctx context.Context, reqCtx *RequestContext) {
	// Mark execution as in progress if it's pending
	if pe.Status == ExecutionStatusPending {
		pe.Status = ExecutionStatusInProgress
		pe.StartedAt = time.Now()
	}

	for i := range pe.Policies {
		p := &pe.Policies[i]
		if p.Status.IsCompleted {
			logrus.Debugf("Skipping completed policy: %s", p.Policy.Name())
			continue
		}

		result := p.Policy.HandleRequest(ctx, reqCtx)
		if result != nil {
			// Process result (e.g., update metadata, handle instructions)
			for k, v := range result.Metadata {
				reqCtx.Metadata[k] = v
			}
			// Handle instructions as needed
		}

		if !p.Policy.AccessRequestBody() {
			p.Status.IsCompleted = true
		}
	}

	// Update execution status if all policies are completed
	if pe.AllCompleted() {
		pe.Status = ExecutionStatusCompleted
		pe.CompletedAt = time.Now()
	}
}
