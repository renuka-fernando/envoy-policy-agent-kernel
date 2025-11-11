package extproc

import (
	"fmt"
	"io"
	"net/url"
	"policy-engine/agent"
	policy "policy-engine/policy"

	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_proc_filter "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	ext_proc_pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	ext_proc_svc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ ext_proc_svc.ExternalProcessorServer = &server{}

type server struct {
}

// Process implements ext_procv3.ExternalProcessorServer.
func (s *server) Process(processServer ext_proc_svc.ExternalProcessor_ProcessServer) error {
	ctx := processServer.Context()
	requestHeadersMap := make(map[string][]string)
	requestContext := &policy.RequestContext{
		Request: &policy.RequestData{
			Body: &policy.BodyData{
				Included:    false,
				StreamIndex: 0,
			},
		},
	}

	responseContext := &policy.ResponseContext{
		Request:  &policy.RequestData{}, // Will be populated from requestContext
		Response: &policy.ResponseData{
			Body: &policy.BodyData{
				Included:    false,
				StreamIndex: 0,
			},
		},
	}

	var policyExecution *policy.PolicyExecution

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := processServer.Recv()
		if err == io.EOF {
			logrus.Debug("EOF")
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}
		logrus.Info(fmt.Sprintf("******** Received Ext Processing Request ********\n%v", req))

		resp := &ext_proc_pb.ProcessingResponse{}
		switch value := req.Request.(type) {
		case *ext_proc_pb.ProcessingRequest_RequestHeaders:
			headers := value.RequestHeaders.Headers.GetHeaders()
			for _, v := range headers {
				requestHeadersMap[v.Key] = append(requestHeadersMap[v.Key], string(v.GetRawValue()))
			}
			requestContext.Request.Headers = requestHeadersMap
			if len(requestHeadersMap[":method"]) > 0 {
				requestContext.Request.Method = requestHeadersMap[":method"][0]
			}
			if len(requestHeadersMap[":path"]) > 0 {
				requestContext.Request.Path, err = url.Parse(requestHeadersMap[":path"][0])
				if err != nil {
					logrus.Error("Error parsing request path", err)
					return err
				}
			}
			if len(requestHeadersMap[":scheme"]) > 0 {
				requestContext.Request.Scheme = requestHeadersMap[":scheme"][0]
			}
			if len(requestHeadersMap[":authority"]) > 0 {
				requestContext.Request.Authority = requestHeadersMap[":authority"][0]
			}
			if len(requestHeadersMap["x-forwarded-for"]) > 0 {
				requestContext.Request.ClientIP = requestHeadersMap["x-forwarded-for"][0]
			}
			if len(requestHeadersMap["x-forwarded-proto"]) > 0 {
				requestContext.Request.ForwardedProto = requestHeadersMap["x-forwarded-proto"][0]
			}

			logrus.Printf("******** Processing Request Headers ******** method: %s, path: %s", requestContext.Request.Method, requestContext.Request.Path)

			requestPolicies, responsePolicies := agent.ListPolicies()
			policyExecution = policy.NewPolicyExecution(requestPolicies, responsePolicies)
			if policyExecution.AccessRequestBody() {
				logrus.Print("******** Accessing Request Body ********")
				resp = &ext_proc_pb.ProcessingResponse{
					ModeOverride: &ext_proc_filter.ProcessingMode{
						RequestBodyMode: ext_proc_filter.ProcessingMode_FULL_DUPLEX_STREAMED,
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending ext proc ProcessingRequest_RequestHeaders response", err)
				}
				continue
			}

			// Execute policies for request headers
			// Note: needMoreData will be false here since we haven't received body yet
			_ = policyExecution.ExecuteRequestPolicies(ctx, requestContext, nil)

		case *ext_proc_pb.ProcessingRequest_RequestBody:
			logrus.Print("******** Processing Request Body ******** body: ", string(value.RequestBody.Body))
			bodyData := &policy.BodyData{
				Included:    true,
				Data:        value.RequestBody.Body,
				EndOfStream: value.RequestBody.EndOfStream,
				StreamIndex: requestContext.Request.Body.StreamIndex + 1,
			}
			result := policyExecution.ExecuteRequestPolicies(ctx, requestContext, bodyData)

			// Check if buffer size exceeded or other fatal error
			if policyExecution.Error != nil {
				logrus.Errorf("Policy execution error: %v", policyExecution.Error)
				resp = &ext_proc_pb.ProcessingResponse{
					Response: &ext_proc_pb.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &ext_proc_pb.ImmediateResponse{
							Status: &type_v3.HttpStatus{
								Code: 413, // Payload Too Large
							},
							Body:    []byte(fmt.Sprintf("Request body buffer limit exceeded: %v", policyExecution.Error)),
							Headers: &ext_proc_pb.HeaderMutation{},
						},
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending error response", err)
				}
				continue
			}

			// If policy requested more data, send empty body mutation to continue streaming
			if result.BufferAction == policy.BufferActionBufferNext {
				logrus.Debug("Policy requested more data, continuing to buffer")
				resp = &ext_proc_pb.ProcessingResponse{
					Response: &ext_proc_pb.ProcessingResponse_RequestBody{
						RequestBody: &ext_proc_pb.BodyResponse{
							Response: &ext_proc_pb.CommonResponse{
								BodyMutation: &ext_proc_pb.BodyMutation{
									Mutation: &ext_proc_pb.BodyMutation_Body{
										Body: []byte{}, // Empty mutation continues streaming
									},
								},
							},
						},
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending continue buffering response", err)
				}
				continue
			}

			// TODO: Process policy results and apply actions (mutations, headers, etc.)

		case *ext_proc_pb.ProcessingRequest_ResponseHeaders:
			headers := value.ResponseHeaders.Headers.GetHeaders()
			responseHeadersMap := make(map[string][]string)
			for _, v := range headers {
				responseHeadersMap[v.Key] = append(responseHeadersMap[v.Key], string(v.GetRawValue()))
			}

			// Populate response context
			responseContext.Response.Headers = responseHeadersMap
			responseContext.Request = requestContext.Request // Link request data
			// Note: responseContext.Metadata will be set to policyExecution.Metadata in ExecuteResponsePolicies

			// Parse status code
			if len(responseHeadersMap[":status"]) > 0 {
				var statusCode int32
				fmt.Sscanf(responseHeadersMap[":status"][0], "%d", &statusCode)
				responseContext.Response.StatusCode = statusCode
			}

			logrus.Printf("******** Processing Response Headers ******** status:%v", responseContext.Response.StatusCode)

			// Check if any policy needs response body access
			if policyExecution != nil && policyExecution.AccessResponseBody() {
				logrus.Print("******** Accessing Response Body ********")
				resp = &ext_proc_pb.ProcessingResponse{
					ModeOverride: &ext_proc_filter.ProcessingMode{
						ResponseBodyMode: ext_proc_filter.ProcessingMode_FULL_DUPLEX_STREAMED,
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending ext proc ProcessingRequest_ResponseHeaders response", err)
				}
				continue
			}

			// If no body access needed, execute response policies now
			// (currently just pass through with example header mutations)
			resp = &ext_proc_pb.ProcessingResponse{
				Response: &ext_proc_pb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &ext_proc_pb.HeadersResponse{
						Response: &ext_proc_pb.CommonResponse{
							HeaderMutation: &ext_proc_pb.HeaderMutation{
								SetHeaders: []*core_v3.HeaderValueOption{
									{
										Header: &core_v3.HeaderValue{
											Key:      "hello",
											RawValue: []byte("world"),
										},
									},
									{
										Header: &core_v3.HeaderValue{
											Key:      "test",
											RawValue: []byte("renuka"),
										},
									},
								},
							},
						},
					},
				},
			}
		case *ext_proc_pb.ProcessingRequest_ResponseBody:
			logrus.Print("******** Processing Response Body ******** body: ", string(value.ResponseBody.Body))
			bodyData := &policy.BodyData{
				Included:    true,
				Data:        value.ResponseBody.Body,
				EndOfStream: value.ResponseBody.EndOfStream,
				StreamIndex: responseContext.Response.Body.StreamIndex + 1,
			}

			result := policyExecution.ExecuteResponsePolicies(ctx, responseContext, bodyData)

			// Check if buffer size exceeded or other fatal error
			if policyExecution.Error != nil {
				logrus.Errorf("Policy execution error: %v", policyExecution.Error)
				resp = &ext_proc_pb.ProcessingResponse{
					Response: &ext_proc_pb.ProcessingResponse_ImmediateResponse{
						ImmediateResponse: &ext_proc_pb.ImmediateResponse{
							Status: &type_v3.HttpStatus{
								Code: 413, // Payload Too Large
							},
							Body:    []byte(fmt.Sprintf("Response body buffer limit exceeded: %v", policyExecution.Error)),
							Headers: &ext_proc_pb.HeaderMutation{},
						},
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending error response", err)
				}
				continue
			}

			// If policy requested more data, send empty body mutation to continue streaming
			if result.BufferAction == policy.BufferActionBufferNext {
				logrus.Debug("Policy requested more data, continuing to buffer")
				resp = &ext_proc_pb.ProcessingResponse{
					Response: &ext_proc_pb.ProcessingResponse_ResponseBody{
						ResponseBody: &ext_proc_pb.BodyResponse{
							Response: &ext_proc_pb.CommonResponse{
								BodyMutation: &ext_proc_pb.BodyMutation{
									Mutation: &ext_proc_pb.BodyMutation_StreamedResponse{
										StreamedResponse: &ext_proc_pb.StreamedBodyResponse{
											Body:        []byte{}, // Empty mutation continues streaming
											EndOfStream: false,
										},
									},
								},
							},
						},
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending continue buffering response", err)
				}
				continue
			}

			// TODO: Process policy results and apply actions (mutations, headers, etc.)
			// For now, just pass through the response body
			resp = &ext_proc_pb.ProcessingResponse{
				Response: &ext_proc_pb.ProcessingResponse_ResponseBody{
					ResponseBody: &ext_proc_pb.BodyResponse{
						Response: &ext_proc_pb.CommonResponse{
							HeaderMutation: &ext_proc_pb.HeaderMutation{
								RemoveHeaders: []string{"x-wso2-response-fault-flag"},
							},
							BodyMutation: &ext_proc_pb.BodyMutation{
								Mutation: &ext_proc_pb.BodyMutation_StreamedResponse{
									StreamedResponse: &ext_proc_pb.StreamedBodyResponse{
										Body:        value.ResponseBody.Body,
										EndOfStream: value.ResponseBody.EndOfStream,
									},
								},
							},
						},
					},
				},
			}
		default:
			logrus.Debug(fmt.Sprintf("Unknown Request type %v\n", value))
		}
		if err := processServer.Send(resp); err != nil {
			logrus.Debug(fmt.Sprintf("send error %v", err))
		}
	}
}

func NewServer() *grpc.Server {
	gs := grpc.NewServer()
	ext_proc_svc.RegisterExternalProcessorServer(gs, &server{})
	return gs
}
