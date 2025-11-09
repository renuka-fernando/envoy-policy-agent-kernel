package policy

// PolicyTask represents a policy that is pending execution or being tracked
// during the policy execution lifecycle. It wraps a Policy implementation
// along with execution state tracking.
type PolicyTask struct {
	// Policy is the user-implemented policy to be executed
	Policy Policy

	// Status tracks the execution state of this policy task
	Status PolicyTaskStatus
}

// PolicyTaskStatus tracks the execution state of a PolicyTask
type PolicyTaskStatus struct {
	// IsCompleted indicates whether this policy task has finished execution
	IsCompleted bool

	// LastExecutedStreamIndex tracks the last stream index where this policy was executed
	// Used to track progress through streaming data or multi-phase execution
	LastExecutedStreamIndex int
}
