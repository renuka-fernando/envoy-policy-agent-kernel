package policy

import (
	"context"

	ext_proc_pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/sirupsen/logrus"
)

func AccessRequestBody(policies []PolicyTask) bool {
	for _, p := range policies {
		if p.Policy.AccessRequestBody() {
			return true
		}
	}
	return false
}

func AccessResponseBody(policies []PolicyTask) bool {
	for _, p := range policies {
		if p.Policy.AccessResponseBody() {
			return true
		}
	}
	return false
}

func ExecuteRequestPolicies(ctx context.Context, policies []PolicyTask, reqCtx *RequestContext) *ext_proc_pb.ProcessingResponse {
	for _, p := range policies {
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
}
