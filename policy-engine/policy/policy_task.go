package policy

// PolicyTask represents a policy that is pending execution or being tracked
// during the policy execution lifecycle. It wraps a Policy implementation
// along with execution state tracking.
type PolicyTask struct {
	// Policy is the user-implemented policy to be executed
	Policy Policy

	// status tracks the execution state of this policy task
	status PolicyTaskStatus
}

// PolicyTaskStatus tracks the execution state of a PolicyTask
type PolicyTaskStatus struct {
	// isCompleted indicates whether this policy task has finished execution
	isCompleted bool

	// lastExecutedStreamIndex tracks the last stream index where this policy was executed
	// Used to track progress through streaming data or multi-phase execution
	lastExecutedStreamIndex int

	// lastExecutedBodyByteIndex tracks the last body byte index where this policy was executed
	// Used when BufferNextChunk is returned to determine which byte slice to provide
	// to the policy on the next execution
	lastExecutedBodyByteIndex int
}
