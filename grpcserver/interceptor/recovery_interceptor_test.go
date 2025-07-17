/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/interop/grpc_testing"

	"github.com/acronis/go-appkit/log/logtest"
)

// RecoveryInterceptorTestSuite is a test suite for Recovery interceptors
type RecoveryInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestRecoveryServerUnaryInterceptor(t *testing.T) {
	suite.Run(t, &RecoveryInterceptorTestSuite{IsUnary: true})
}

func TestRecoveryServerStreamInterceptor(t *testing.T) {
	suite.Run(t, &RecoveryInterceptorTestSuite{IsUnary: false})
}

func (s *RecoveryInterceptorTestSuite) TestRecoveryServerInterceptorWithStackSize() {
	tests := []struct {
		name      string
		options   []RecoveryOption
		wantStack bool
	}{
		{
			name:      "Default stack size",
			options:   nil,
			wantStack: true, // Default stack size is > 0
		},
		{
			name:      "Custom stack size",
			options:   []RecoveryOption{WithRecoveryStackSize(4096)},
			wantStack: true,
		},
		{
			name:      "Zero stack size",
			options:   []RecoveryOption{WithRecoveryStackSize(0)},
			wantStack: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := logtest.NewRecorder()

			chainUnaryInterceptor := grpc.ChainUnaryInterceptor(
				RequestIDUnaryInterceptor(),
				LoggingUnaryInterceptor(logger),
				RecoveryUnaryInterceptor(tt.options...),
			)
			chainStreamInterceptor := grpc.ChainStreamInterceptor(
				RequestIDStreamInterceptor(),
				LoggingStreamInterceptor(logger),
				RecoveryStreamInterceptor(tt.options...),
			)
			svc, client, closeSvc, err := startTestService(
				[]grpc.ServerOption{chainUnaryInterceptor, chainStreamInterceptor}, nil)
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
				panic("test panic")
			})
			svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
				panic("test panic")
			})

			if s.IsUnary {
				_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
				s.Require().ErrorIs(err, InternalError)
			} else {
				stream, err := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(err)
				_, err = stream.Recv()
				s.Require().ErrorIs(err, InternalError)
			}

			// Should have 2 log entries: panic log and request finished log
			s.Require().Equal(2, len(logger.Entries()))
			panicEntry := logger.Entries()[0]
			s.Require().Contains(panicEntry.Text, "Panic: test panic")

			stackField := getLogFieldAsString(panicEntry, "stack")
			if tt.wantStack {
				s.Require().NotEmpty(stackField)
				s.Require().Contains(stackField, "panic") // Stack trace should contain panic info
			} else {
				// When stack size is 0, stack field should be empty
				s.Require().Empty(stackField)
			}
		})
	}
}

func (s *RecoveryInterceptorTestSuite) TestRecoveryServerInterceptorNoLogger() {
	svc, client, closeSvc, err := startTestService([]grpc.ServerOption{
		grpc.UnaryInterceptor(RecoveryUnaryInterceptor()),
		grpc.StreamInterceptor(RecoveryStreamInterceptor()),
	}, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
		panic("test panic")
	})
	svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
		panic("test panic")
	})

	if s.IsUnary {
		_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().ErrorIs(err, InternalError)
		// Should not panic or cause issues when no logger is present
	} else {
		stream, err := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(err)

		_, err = stream.Recv()
		s.Require().ErrorIs(err, InternalError)
		// Should not panic or cause issues when no logger is present
	}
}
