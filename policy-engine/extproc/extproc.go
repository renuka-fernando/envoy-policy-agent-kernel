package extproc

import (
	"fmt"
	"io"
	"net/url"
	"policy-engine/agent"
	policy "policy-engine/policy"

	core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_procv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var _ ext_proc_v3.ExternalProcessorServer = &server{}

type server struct {
}

// Process implements ext_procv3.ExternalProcessorServer.
func (s *server) Process(processServer ext_proc_v3.ExternalProcessor_ProcessServer) error {
	ctx := processServer.Context()
	requestHeadersMap := make(map[string][]string)
	requestContext := &policy.RequestContext{
		Metadata: make(map[string]string),
		Request: &policy.RequestData{
			BodyIncluded: false,
		},
	}

	var policyList []policy.Policy

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

		resp := &pb.ProcessingResponse{}
		switch value := req.Request.(type) {
		case *pb.ProcessingRequest_RequestHeaders:
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

			policyList = agent.ListPolicies()
			if policy.AccessRequestBody(policyList) {
				logrus.Print("******** Accessing Request Body ********")
				resp = &pb.ProcessingResponse{
					ModeOverride: &ext_procv3.ProcessingMode{
						RequestBodyMode: ext_procv3.ProcessingMode_FULL_DUPLEX_STREAMED,
					},
				}
				if err := processServer.Send(resp); err != nil {
					logrus.Error("Error sending ext proc ProcessingRequest_RequestHeaders response", err)
				}
				continue
			}

			policy.ExecuteRequestPolicies(ctx, policyList, requestContext)

		case *pb.ProcessingRequest_RequestBody:
			logrus.Print("******** Processing Request Body ******** body: ", string(value.RequestBody.Body))
			resp = &pb.ProcessingResponse{
				DynamicMetadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"envoy.filters.http.ext_proc": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"my_dynamic_key": {
											Kind: &structpb.Value_StringValue{
												StringValue: "my_dynamic_value",
											},
										},
									},
								},
							},
						},
					},
				},
				Response: &pb.ProcessingResponse_RequestBody{
					RequestBody: &pb.BodyResponse{
						Response: &pb.CommonResponse{
							HeaderMutation: &pb.HeaderMutation{
								RemoveHeaders: []string{
									"remove-this-header",
								},
								// SetHeaders: []*core_v3.HeaderValueOption{
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      "hello",
								// 			RawValue: []byte("world!!!"),
								// 		},
								// 	},
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      "content-length",
								// 			RawValue: []byte("11"),
								// 		},
								// 	},
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key: ":method",
								// 			// Value: "PUT",
								// 			RawValue: []byte("PUT"),
								// 		},
								// 	},
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      ":path",
								// 			RawValue: []byte("/foo"),
								// 		},
								// 	},
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      "foo",
								// 			RawValue: []byte("bar1"),
								// 		},
								// 	},
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      "foo",
								// 			RawValue: []byte("bar2,bar3"),
								// 		},
								// 	},
								// },
							},
							BodyMutation: &pb.BodyMutation{
								// Mutation: &pb.BodyMutation_Body{
								// 	Body: []byte("Hello World"),
								// },
								Mutation: &pb.BodyMutation_StreamedResponse{
									StreamedResponse: &pb.StreamedBodyResponse{
										Body:        value.RequestBody.Body,
										EndOfStream: value.RequestBody.EndOfStream,
									},
								},
							},
							ClearRouteCache: false,
						},
					},
				},
			}
		case *pb.ProcessingRequest_ResponseHeaders:
			headers := value.ResponseHeaders.Headers.GetHeaders()
			headersMap := make(map[string]string)
			for _, v := range headers {
				headersMap[v.Key] = string(v.GetRawValue())
			}

			status := headersMap[":status"]
			_, isFaultFlow := headersMap["x-wso2-response-fault-flag"]
			if isFaultFlow {
				logrus.Print("ERROR FLOW")
			}

			logrus.Print(fmt.Sprintf("******** Processing Response Headers ******** status:%v", status))
			resp = &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &pb.HeadersResponse{
						Response: &pb.CommonResponse{
							HeaderMutation: &pb.HeaderMutation{
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
		case *pb.ProcessingRequest_ResponseBody:
			logrus.Print("******** Processing Response Body ******** body: ", string(value.ResponseBody.Body))
			logrus.Printf("******** Processing Response Body ******** validate: %v", value.ResponseBody.Validate())

			// body := "Hello World!! Response!!!"
			resp = &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_ResponseBody{
					ResponseBody: &pb.BodyResponse{
						Response: &pb.CommonResponse{
							// BodyMutation: &pb.BodyMutation{
							// 	Mutation: &pb.BodyMutation_Body{
							// 		Body: []byte(body),
							// 	},
							// },
							HeaderMutation: &pb.HeaderMutation{
								// SetHeaders: []*core_v3.HeaderValueOption{
								// 	{
								// 		Header: &core_v3.HeaderValue{
								// 			Key:      "Content-Length",
								// 			RawValue: []byte(fmt.Sprint(len(body))),
								// 		},
								// 	},
								// },
								RemoveHeaders: []string{"x-wso2-response-fault-flag"},
							},
							BodyMutation: &pb.BodyMutation{
								Mutation: &pb.BodyMutation_StreamedResponse{
									StreamedResponse: &pb.StreamedBodyResponse{
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
	ext_proc_v3.RegisterExternalProcessorServer(gs, &server{})
	return gs
}
