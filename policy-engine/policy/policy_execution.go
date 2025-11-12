package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// PolicyExecution represents the execution state and metadata for the complete
// request-response lifecycle. It manages separate policy sets for request and
// response phases, tracks their execution status, and maintains lifecycle metadata
// that policies can read, set, or delete throughout the entire lifecycle.
type PolicyExecution struct {
	// RequestPolicies contains policy tasks executed during the request phase
	RequestPolicies []PolicyTask

	// ResponsePolicies contains policy tasks executed during the response phase
	ResponsePolicies []PolicyTask

	// Metadata holds lifecycle metadata that persists across request and response phases.
	// Policies can set, read, and delete metadata entries throughout the lifecycle.
	Metadata map[string]string

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

	// requestBodyBuffer accumulates request body chunks for incremental processing
	requestBodyBuffer *BodyBuffer

	// responseBodyBuffer accumulates response body chunks for incremental processing
	responseBodyBuffer *BodyBuffer

	// waitingForFullRequestBuffer indicates we're buffering the full request body before executing
	waitingForFullRequestBuffer bool

	// waitingForFullResponseBuffer indicates we're buffering the full response body before executing
	waitingForFullResponseBuffer bool
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

// BufferAction represents the buffering action required after policy execution
type BufferAction string

const (
	// BufferActionNone indicates no buffering action is needed
	BufferActionNone BufferAction = "none"

	// BufferActionBufferNext indicates the policy needs more data chunks
	BufferActionBufferNext BufferAction = "buffer_next"

	// BufferActionFullBuffer indicates the full buffer is needed before proceeding
	BufferActionFullBuffer BufferAction = "full_buffer"
)

// PolicyExecutionResult represents the result of executing policies
type PolicyExecutionResult struct {
	// BufferAction indicates what buffering action is required
	BufferAction BufferAction

	// RequestResults contains the list of request policy results from execution
	// If an ImmediateResponse is encountered, this will contain only that result
	RequestResults []*RequestResult

	// ResponseResults contains the list of response policy results from execution
	ResponseResults []*ResponseResult
}

// BodyBuffer accumulates body chunks across streaming requests/responses.
// It allows policies to incrementally consume bytes and request more data.
type BodyBuffer struct {
	data        []byte // Accumulated bytes across all chunks
	streamIndex int    // Last stream index added to buffer
	endOfStream bool   // Whether the final chunk has been received
	maxSize     int    // Maximum buffer size in bytes
}

// BufferSizeExceededError represents an error when the body buffer size limit is exceeded
type BufferSizeExceededError struct {
	CurrentSize int
	ChunkSize   int
	MaxSize     int
	Err         error
}

func (e *BufferSizeExceededError) Error() string {
	return fmt.Sprintf("buffer size limit exceeded: current=%d, chunk=%d, max=%d",
		e.CurrentSize, e.ChunkSize, e.MaxSize)
}

func (e *BufferSizeExceededError) Unwrap() error {
	return e.Err
}

// BufferMoreDataAtEndOfStreamError represents an error when a policy requests more data
// but the stream has already ended
type BufferMoreDataAtEndOfStreamError struct {
	PolicyName string
	Phase      string // "request" or "response"
}

func (e *BufferMoreDataAtEndOfStreamError) Error() string {
	return fmt.Sprintf("policy %s requested more data but %s stream has already ended",
		e.PolicyName, e.Phase)
}

// DefaultMaxBufferSize is the default maximum buffer size (1MB)
const DefaultMaxBufferSize = 1024 * 1024

// NewBodyBuffer creates a new BodyBuffer with the specified maximum size
func NewBodyBuffer(maxSize int) *BodyBuffer {
	return &BodyBuffer{
		data:        make([]byte, 0),
		streamIndex: -1,
		endOfStream: false,
		maxSize:     maxSize,
	}
}

// Append adds a chunk of data to the buffer
// Returns a BufferSizeExceededError if appending would exceed maxSize
func (b *BodyBuffer) Append(chunk []byte, streamIdx int, eos bool) error {
	if len(b.data)+len(chunk) > b.maxSize {
		return &BufferSizeExceededError{
			CurrentSize: len(b.data),
			ChunkSize:   len(chunk),
			MaxSize:     b.maxSize,
		}
	}
	b.data = append(b.data, chunk...)
	b.streamIndex = streamIdx
	b.endOfStream = eos
	return nil
}

// GetSlice returns a slice of the buffer starting from the given byte index
func (b *BodyBuffer) GetSlice(fromIndex int) []byte {
	if fromIndex >= len(b.data) {
		return []byte{}
	}
	return b.data[fromIndex:]
}

// Size returns the total number of bytes currently buffered
func (b *BodyBuffer) Size() int {
	return len(b.data)
}

// StreamIndex returns the last stream index added to the buffer
func (b *BodyBuffer) StreamIndex() int {
	return b.streamIndex
}

// EndOfStream returns whether the final chunk has been received
func (b *BodyBuffer) EndOfStream() bool {
	return b.endOfStream
}

// Reset clears the buffer and resets its state
func (b *BodyBuffer) Reset() {
	b.data = b.data[:0]
	b.streamIndex = -1
	b.endOfStream = false
}

// NewPolicyExecution creates a new PolicyExecution with separate request and response policies.
// It wraps each policy in a PolicyTask with initial status and initializes lifecycle metadata.
func NewPolicyExecution(requestPolicies []Policy, responsePolicies []Policy) *PolicyExecution {
	requestTasks := make([]PolicyTask, len(requestPolicies))
	for i, p := range requestPolicies {
		requestTasks[i] = PolicyTask{
			Policy: p,
			status: PolicyTaskStatus{
				isCompleted:             false,
				lastExecutedStreamIndex: 0,
			},
		}
	}

	responseTasks := make([]PolicyTask, len(responsePolicies))
	for i, p := range responsePolicies {
		responseTasks[i] = PolicyTask{
			Policy: p,
			status: PolicyTaskStatus{
				isCompleted:             false,
				lastExecutedStreamIndex: 0,
			},
		}
	}

	pe := &PolicyExecution{
		RequestPolicies:    requestTasks,
		ResponsePolicies:   responseTasks,
		Metadata:           make(map[string]string),
		Status:             ExecutionStatusPending,
		requestBodyBuffer:  NewBodyBuffer(DefaultMaxBufferSize),
		responseBodyBuffer: NewBodyBuffer(DefaultMaxBufferSize),
	}

	pe.accessRequestBody = pe.checkAccessRequestBody()
	pe.accessResponseBody = pe.checkAccessResponseBody()

	return pe
}

// checkAccessRequestBody checks if any request policy needs to access the request body
func (pe *PolicyExecution) checkAccessRequestBody() bool {
	for _, p := range pe.RequestPolicies {
		if p.Policy.AccessRequestBody() {
			return true
		}
	}
	return false
}

// checkAccessResponseBody checks if any response policy needs to access the response body
func (pe *PolicyExecution) checkAccessResponseBody() bool {
	for _, p := range pe.ResponsePolicies {
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

// AllCompleted checks if all policy tasks (both request and response) have completed
func (pe *PolicyExecution) AllCompleted() bool {
	for _, p := range pe.RequestPolicies {
		if !p.status.isCompleted {
			return false
		}
	}
	for _, p := range pe.ResponsePolicies {
		if !p.status.isCompleted {
			return false
		}
	}
	return true
}

// AnyCompleted checks if any policy task (request or response) has completed
func (pe *PolicyExecution) AnyCompleted() bool {
	for _, p := range pe.RequestPolicies {
		if p.status.isCompleted {
			return true
		}
	}
	for _, p := range pe.ResponsePolicies {
		if p.status.isCompleted {
			return true
		}
	}
	return false
}

// ExecuteRequestPolicies executes all request policies for request processing.
// It handles streaming body data by accumulating chunks in a buffer and providing
// incremental slices to policies based on their consumption progress.
// The request context uses the PolicyExecution's lifecycle metadata directly,
// so policies can directly set, modify, or delete metadata entries.
// bodyData contains the current chunk of body data to process (can be nil if no body).
// Returns a PolicyExecutionResult with execution details.
func (pe *PolicyExecution) ExecuteRequestPolicies(ctx context.Context, reqCtx *RequestContext, bodyData *BodyData) *PolicyExecutionResult {
	// Mark execution as in progress if it's pending
	if pe.Status == ExecutionStatusPending {
		pe.Status = ExecutionStatusInProgress
		pe.StartedAt = time.Now()
	}

	// Use PolicyExecution's lifecycle metadata directly
	reqCtx.Metadata = pe.Metadata

	// If body data is provided, append to buffer
	if bodyData != nil && bodyData.Included {
		err := pe.requestBodyBuffer.Append(
			bodyData.Data,
			bodyData.StreamIndex,
			bodyData.EndOfStream,
		)
		if err != nil {
			// Buffer size exceeded - store error and mark as failed
			pe.Error = err
			pe.Status = ExecutionStatusFailed
			pe.CompletedAt = time.Now()
			return &PolicyExecutionResult{
				BufferAction:   BufferActionNone,
				RequestResults: []*RequestResult{},
			}
		}
	}

	// If waiting for full buffer and not end of stream, just continue buffering
	if pe.waitingForFullRequestBuffer && !pe.requestBodyBuffer.EndOfStream() {
		return &PolicyExecutionResult{
			BufferAction:   BufferActionFullBuffer,
			RequestResults: []*RequestResult{},
		}
	}

	// Reset waiting flag if we reached end of stream
	if pe.waitingForFullRequestBuffer && pe.requestBodyBuffer.EndOfStream() {
		pe.waitingForFullRequestBuffer = false
	}

	var requestResults []*RequestResult

	for i := range pe.RequestPolicies {
		p := &pe.RequestPolicies[i]
		if p.status.isCompleted {
			// Skip already completed policies
			// Example: policies that don't access body and process only headers.
			logrus.Debugf("Skipping completed policy: %s", p.Policy.Name())
			continue
		}

		// Policies that don't access request body execute once and complete
		if !p.Policy.AccessRequestBody() {
			result := p.Policy.HandleRequest(ctx, reqCtx)
			p.status.isCompleted = true

			// Collect result if returned
			if result != nil {
				requestResults = append(requestResults, result)

				// If ImmediateResponse, override results and return immediately
				if _, isImmediateResponse := result.Action.(*ImmediateResponse); isImmediateResponse {
					return &PolicyExecutionResult{
						BufferAction:   BufferActionNone,
						RequestResults: []*RequestResult{result},
					}
				}
			}
			continue
		}

		// For policies that access the body, provide buffered data slice
		bodySlice := pe.requestBodyBuffer.GetSlice(p.status.lastExecutedBodyByteIndex)

		reqCtx.Request.Body = &BodyData{
			Data:        bodySlice,
			Included:    len(bodySlice) > 0 || pe.requestBodyBuffer.EndOfStream(),
			EndOfStream: pe.requestBodyBuffer.EndOfStream(),
			StreamIndex: pe.requestBodyBuffer.StreamIndex(),
		}

		result := p.Policy.HandleRequest(ctx, reqCtx)
		if result != nil {
			// Check what action the policy returned
			switch action := result.Action.(type) {
			case *ImmediateResponse:
				// Immediate response - return immediately with only this result
				return &PolicyExecutionResult{
					BufferAction:   BufferActionNone,
					RequestResults: []*RequestResult{result},
				}

			case *BufferNextChunk, *BufferFullBody:
				// Error if policy requests more data but stream has already ended
				if pe.requestBodyBuffer.EndOfStream() {
					pe.Error = &BufferMoreDataAtEndOfStreamError{
						PolicyName: p.Policy.Name(),
						Phase:      "request",
					}
					pe.Status = ExecutionStatusFailed
					pe.CompletedAt = time.Now()
					return &PolicyExecutionResult{
						BufferAction:   BufferActionNone,
						RequestResults: requestResults,
					}
				}

				// Handle the specific buffer action
				if _, isBufferNext := action.(*BufferNextChunk); isBufferNext {
					logrus.Debugf("Policy %s requested BufferNextChunk", p.Policy.Name())
					return &PolicyExecutionResult{
						BufferAction:   BufferActionBufferNext,
						RequestResults: requestResults,
					}
				} else {
					logrus.Debugf("Policy %s requested BufferFullBody", p.Policy.Name())
					pe.waitingForFullRequestBuffer = true
					return &PolicyExecutionResult{
						BufferAction:   BufferActionFullBuffer,
						RequestResults: requestResults,
					}
				}

			default:
				// Collect the result
				requestResults = append(requestResults, result)

				// Update the byte index to current buffer size (policy consumed data)
				p.status.lastExecutedBodyByteIndex = pe.requestBodyBuffer.Size()
				p.status.lastExecutedStreamIndex = pe.requestBodyBuffer.StreamIndex()
			}
		}

		// Mark policy as completed if we've reached end of stream
		if pe.requestBodyBuffer.EndOfStream() {
			p.status.isCompleted = true
		}
	}

	// Update execution status if all policies are completed
	if pe.AllCompleted() {
		pe.Status = ExecutionStatusCompleted
		pe.CompletedAt = time.Now()
	}

	return &PolicyExecutionResult{
		BufferAction:   BufferActionNone,
		RequestResults: requestResults,
	}
}

// ExecuteResponsePolicies executes all response policies for response processing.
// It handles streaming body data by accumulating chunks in a buffer and providing
// incremental slices to policies based on their consumption progress.
// The response context uses the PolicyExecution's lifecycle metadata directly,
// so policies can directly set, modify, or delete metadata entries.
// bodyData contains the current chunk of body data to process (can be nil if no body).
// Returns a PolicyExecutionResult with execution details.
func (pe *PolicyExecution) ExecuteResponsePolicies(ctx context.Context, respCtx *ResponseContext, bodyData *BodyData) *PolicyExecutionResult {
	// Use PolicyExecution's lifecycle metadata directly
	respCtx.Metadata = pe.Metadata

	// If body data is provided, append to buffer
	if bodyData != nil && bodyData.Included {
		err := pe.responseBodyBuffer.Append(
			bodyData.Data,
			bodyData.StreamIndex,
			bodyData.EndOfStream,
		)
		if err != nil {
			// Buffer size exceeded - store error and mark as failed
			pe.Error = err
			pe.Status = ExecutionStatusFailed
			pe.CompletedAt = time.Now()
			return &PolicyExecutionResult{
				BufferAction:    BufferActionNone,
				ResponseResults: []*ResponseResult{},
			}
		}
	}

	// If waiting for full buffer and not end of stream, just continue buffering
	if pe.waitingForFullResponseBuffer && !pe.responseBodyBuffer.EndOfStream() {
		return &PolicyExecutionResult{
			BufferAction:    BufferActionFullBuffer,
			ResponseResults: []*ResponseResult{},
		}
	}

	// Reset waiting flag if we reached end of stream
	if pe.waitingForFullResponseBuffer && pe.responseBodyBuffer.EndOfStream() {
		pe.waitingForFullResponseBuffer = false
	}

	var responseResults []*ResponseResult

	for i := range pe.ResponsePolicies {
		p := &pe.ResponsePolicies[i]

		// Policies that don't access response body execute once and complete
		if !p.Policy.AccessResponseBody() {
			result := p.Policy.HandleResponse(ctx, respCtx)

			// Collect result if returned
			if result != nil {
				responseResults = append(responseResults, result)
			}
			continue
		}

		// For policies that access the body, provide buffered data slice
		bodySlice := pe.responseBodyBuffer.GetSlice(p.status.lastExecutedBodyByteIndex)

		// Create a temporary body data with the slice for this policy
		originalBody := respCtx.Response.Body
		respCtx.Response.Body = &BodyData{
			Data:        bodySlice,
			Included:    len(bodySlice) > 0 || pe.responseBodyBuffer.EndOfStream(),
			EndOfStream: pe.responseBodyBuffer.EndOfStream(),
			StreamIndex: pe.responseBodyBuffer.StreamIndex(),
		}

		result := p.Policy.HandleResponse(ctx, respCtx)

		// Restore original body reference
		respCtx.Response.Body = originalBody

		if result != nil {
			// Check what action the policy returned
			switch action := result.Action.(type) {
			case *BufferNextChunk, *BufferFullBody:
				// Error if policy requests more data but stream has already ended
				if pe.responseBodyBuffer.EndOfStream() {
					pe.Error = &BufferMoreDataAtEndOfStreamError{
						PolicyName: p.Policy.Name(),
						Phase:      "response",
					}
					pe.Status = ExecutionStatusFailed
					pe.CompletedAt = time.Now()
					return &PolicyExecutionResult{
						BufferAction:    BufferActionNone,
						ResponseResults: responseResults,
					}
				}

				// Handle the specific buffer action
				if _, isBufferNext := action.(*BufferNextChunk); isBufferNext {
					logrus.Debugf("Policy %s requested BufferNextChunk", p.Policy.Name())
					return &PolicyExecutionResult{
						BufferAction:    BufferActionBufferNext,
						ResponseResults: responseResults,
					}
				} else {
					logrus.Debugf("Policy %s requested BufferFullBody", p.Policy.Name())
					pe.waitingForFullResponseBuffer = true
					return &PolicyExecutionResult{
						BufferAction:    BufferActionFullBuffer,
						ResponseResults: responseResults,
					}
				}

			default:
				// Collect the result
				responseResults = append(responseResults, result)

				// Update the byte index to current buffer size (policy consumed data)
				p.status.lastExecutedBodyByteIndex = pe.responseBodyBuffer.Size()
				p.status.lastExecutedStreamIndex = pe.responseBodyBuffer.StreamIndex()
			}
		}
	}

	return &PolicyExecutionResult{
		BufferAction:    BufferActionNone,
		ResponseResults: responseResults,
	}
}
