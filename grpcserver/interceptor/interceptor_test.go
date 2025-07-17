/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/interop/grpc_testing"
)

type testService struct {
	grpc_testing.UnimplementedTestServiceServer
	lastCtx                    context.Context
	unaryCallHandler           func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error)
	streamingOutputCallHandler func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error
}

func (s *testService) UnaryCall(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
	s.lastCtx = ctx
	if s.unaryCallHandler != nil {
		return s.unaryCallHandler(ctx, req)
	}
	return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
}

func (s *testService) StreamingOutputCall(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
	s.lastCtx = stream.Context()
	if s.streamingOutputCallHandler != nil {
		return s.streamingOutputCallHandler(req, stream)
	}
	return stream.Send(&grpc_testing.StreamingOutputCallResponse{
		Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
	})
}

func (s *testService) SwitchUnaryCallHandler(
	handler func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error),
) {
	s.unaryCallHandler = handler
}

func (s *testService) SwitchStreamingOutputCallHandler(
	handler func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error,
) {
	s.streamingOutputCallHandler = handler
}

func (s *testService) Reset() {
	s.lastCtx = nil
	s.unaryCallHandler = nil
	s.streamingOutputCallHandler = nil
}

func startTestService(
	serverOpts []grpc.ServerOption,
	dialOpts []grpc.DialOption,
) (svc *testService, client grpc_testing.TestServiceClient, closeFn func() error, err error) {
	svc = &testService{}
	var clientConn *grpc.ClientConn
	if _, clientConn, closeFn, err = newTestServerAndClient(serverOpts, dialOpts, func(s *grpc.Server) {
		grpc_testing.RegisterTestServiceServer(s, svc)
	}); err != nil {
		return nil, nil, nil, err
	}
	return svc, grpc_testing.NewTestServiceClient(clientConn), closeFn, nil
}

func newTestServerAndClient(
	serverOpts []grpc.ServerOption, dialOpts []grpc.DialOption, registerFn func(s *grpc.Server),
) (server *grpc.Server, clientConn *grpc.ClientConn, closeFn func() error, err error) {
	srv := grpc.NewServer(serverOpts...)
	registerFn(srv)
	ln, lnErr := net.Listen("tcp", "localhost:0")
	if lnErr != nil {
		return nil, nil, nil, fmt.Errorf("listen: %w", lnErr)
	}
	serveResult := make(chan error)
	go func() {
		serveResult <- srv.Serve(ln)
	}()
	defer func() {
		if err != nil {
			srv.Stop()
			if srvErr := <-serveResult; srvErr != nil {
				err = fmt.Errorf("serve: %w; %w", srvErr, err)
			}
		}
	}()

	// Create client connection with insecure credentials
	clientConn, dialErr := grpc.NewClient(ln.Addr().String(),
		append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))...,
	)
	if dialErr != nil {
		return nil, nil, nil, fmt.Errorf("dial: %w", dialErr)
	}
	return srv, clientConn, func() error {
		mErr := clientConn.Close()
		srv.GracefulStop()
		return errors.Join(mErr, <-serveResult)
	}, nil
}
