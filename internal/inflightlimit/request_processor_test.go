/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package inflightlimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// SlotsProviderTestSuite contains tests for slots provider functionality
type SlotsProviderTestSuite struct {
	suite.Suite
}

func TestSlotsProvider(t *testing.T) {
	suite.Run(t, new(SlotsProviderTestSuite))
}

func (s *SlotsProviderTestSuite) TestNewSlotsProvider() {
	tests := []struct {
		name         string
		limit        int
		backlogLimit int
		maxKeys      int
		wantErr      bool
	}{
		{
			name:         "with max keys",
			limit:        5,
			backlogLimit: 10,
			maxKeys:      100,
			wantErr:      false,
		},
		{
			name:         "zero max keys",
			limit:        5,
			backlogLimit: 10,
			maxKeys:      0,
			wantErr:      false,
		},
		{
			name:         "zero backlog limit",
			limit:        5,
			backlogLimit: 0,
			maxKeys:      100,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			provider, err := newSlotsProvider(tt.limit, tt.backlogLimit, tt.maxKeys)
			if tt.wantErr {
				s.Error(err)
				s.Nil(provider)
				return
			}
			s.NoError(err)
			s.NotNil(provider)

			inFlightSlots, backlogSlots := provider("test-key")
			s.NotNil(inFlightSlots)
			s.NotNil(backlogSlots)
			s.Equal(tt.limit, cap(inFlightSlots))
			s.Equal(tt.limit+tt.backlogLimit, cap(backlogSlots))
		})
	}
}

func (s *SlotsProviderTestSuite) TestSlotsProvider_SameKeyReturnsSameSlots() {
	provider, err := newSlotsProvider(5, 10, 0) // maxKeys = 0 means single shared slots
	s.NoError(err)
	key := "test-key"

	inFlightSlots1, backlogSlots1 := provider(key)
	inFlightSlots2, backlogSlots2 := provider(key)

	s.Equal(inFlightSlots1, inFlightSlots2, "same key should return same in-flight slots when maxKeys=0")
	s.Equal(backlogSlots1, backlogSlots2, "same key should return same backlog slots when maxKeys=0")
}

func (s *SlotsProviderTestSuite) TestSlotsProvider_DifferentKeysWithLRU() {
	provider, err := newSlotsProvider(5, 10, 100)
	s.NoError(err)
	key1 := "test-key-1"
	key2 := "test-key-2"

	inFlightSlots1, backlogSlots1 := provider(key1)
	inFlightSlots2, backlogSlots2 := provider(key2)

	s.NotEqual(inFlightSlots1, inFlightSlots2, "different keys should have different in-flight slots")
	s.NotEqual(backlogSlots1, backlogSlots2, "different keys should have different backlog slots")
	s.Equal(5, cap(inFlightSlots1))
	s.Equal(5, cap(inFlightSlots2))
	s.Equal(15, cap(backlogSlots1)) // limit + backlogLimit
	s.Equal(15, cap(backlogSlots2))
}

// RequestProcessorTestSuite contains tests for RequestProcessor
type RequestProcessorTestSuite struct {
	suite.Suite
}

func TestRequestProcessor(t *testing.T) {
	suite.Run(t, new(RequestProcessorTestSuite))
}

func (s *RequestProcessorTestSuite) TestNewRequestProcessor() {
	tests := []struct {
		name           string
		limit          int
		backlogParams  BacklogParams
		dryRun         bool
		wantErr        bool
		expectedErrMsg string
	}{
		{
			name:  "valid parameters",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun:  false,
			wantErr: false,
		},
		{
			name:  "zero backlog limit",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			dryRun:  false,
			wantErr: false,
		},
		{
			name:  "zero limit",
			limit: 0,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun:         false,
			wantErr:        true,
			expectedErrMsg: "limit should be positive",
		},
		{
			name:  "negative limit",
			limit: -1,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun:         false,
			wantErr:        true,
			expectedErrMsg: "limit should be positive",
		},
		{
			name:  "negative backlog limit",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   -1,
				Timeout: time.Second,
			},
			dryRun:         false,
			wantErr:        true,
			expectedErrMsg: "backlog limit should not be negative",
		},
		{
			name:  "negative max keys",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: -1,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun:         false,
			wantErr:        true,
			expectedErrMsg: "max keys for backlog should not be negative",
		},
		{
			name:  "zero timeout uses default",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: 0,
			},
			dryRun:  false,
			wantErr: false,
		},
		{
			name:  "dry run mode",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			processor, err := NewRequestProcessor(tt.limit, tt.backlogParams, tt.dryRun)
			if tt.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tt.expectedErrMsg)
				s.Nil(processor)
				return
			}
			s.NoError(err)
			s.NotNil(processor)
			s.Equal(tt.limit, processor.limit)
			s.NotNil(processor.getSlots)
			s.Equal(tt.dryRun, processor.dryRun)
			expectedTimeout := tt.backlogParams.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = DefaultInFlightLimitBacklogTimeout
			}
			s.Equal(expectedTimeout, processor.backlogTimeout)
		})
	}
}

func (s *RequestProcessorTestSuite) TestProcessRequest() {
	tests := []struct {
		name                string
		limit               int
		backlogParams       BacklogParams
		dryRun              bool
		requestHandler      *mockRequestHandler
		expectedError       string
		expectedExecuteCall bool
	}{
		{
			name:  "bypass in-flight limiting",
			limit: 1,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			dryRun: false,
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: true,
			},
			expectedExecuteCall: true,
		},
		{
			name:  "allow request immediately",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun: false,
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: false,
			},
			expectedExecuteCall: true,
		},
		{
			name:  "get key error",
			limit: 5,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun: false,
			requestHandler: &mockRequestHandler{
				keyError: errors.New("key error"),
			},
			expectedError:       "get key for in-flight limit: key error",
			expectedExecuteCall: false,
		},
		{
			name:  "dry run mode allows immediate execution",
			limit: 1,
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			dryRun: true,
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: false,
			},
			expectedExecuteCall: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			processor, err := NewRequestProcessor(tt.limit, tt.backlogParams, tt.dryRun)
			s.NoError(err)

			err = processor.ProcessRequest(tt.requestHandler)

			if tt.expectedError != "" {
				s.Error(err)
				s.Contains(err.Error(), tt.expectedError)
			} else {
				s.NoError(err)
			}

			s.Equal(tt.expectedExecuteCall, tt.requestHandler.executeCalled)
		})
	}
}

func (s *RequestProcessorTestSuite) TestProcessRequest_BacklogFull() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   0, // No backlog
		Timeout: time.Second,
	}, false)
	s.NoError(err)

	// Fill the single in-flight slot and the backlog slot (capacity is limit + backlogLimit = 1 + 0 = 1)
	inFlightSlots, backlogSlots := processor.getSlots("test-key")
	inFlightSlots <- struct{}{}
	backlogSlots <- struct{}{} // Fill the single backlog slot

	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	err = processor.ProcessRequest(requestHandler)
	s.NoError(err)
	s.False(requestHandler.executeCalled)
	s.True(requestHandler.onRejectCalled)
	s.Equal("test-key", requestHandler.lastParams.Key)
	s.False(requestHandler.lastParams.RequestBacklogged)
}

func (s *RequestProcessorTestSuite) TestProcessRequest_WithBacklog() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   2,
		Timeout: time.Millisecond * 100,
	}, false)
	s.NoError(err)

	// Fill the single in-flight slot
	inFlightSlots, _ := processor.getSlots("test-key")
	inFlightSlots <- struct{}{}

	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	// Start a goroutine to release the in-flight slot after a short delay
	go func() {
		time.Sleep(time.Millisecond * 50)
		<-inFlightSlots
	}()

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	s.NoError(err)
	s.True(requestHandler.executeCalled)
	s.GreaterOrEqual(duration, time.Millisecond*40) // Allow some tolerance
}

func (s *RequestProcessorTestSuite) TestProcessRequest_BacklogTimeout() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   2,
		Timeout: time.Millisecond * 100,
	}, false)
	s.NoError(err)

	// Fill the single in-flight slot
	inFlightSlots, _ := processor.getSlots("test-key")
	inFlightSlots <- struct{}{}

	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	s.NoError(err)
	s.False(requestHandler.executeCalled)
	s.True(requestHandler.onRejectCalled)
	s.Equal("test-key", requestHandler.lastParams.Key)
	s.True(requestHandler.lastParams.RequestBacklogged)
	s.GreaterOrEqual(duration, time.Millisecond*90) // Allow tolerance
	s.LessOrEqual(duration, time.Millisecond*200)
}

func (s *RequestProcessorTestSuite) TestProcessRequest_ContextCancellation() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   2,
		Timeout: time.Second * 10,
	}, false)
	s.NoError(err)

	// Fill the single in-flight slot
	inFlightSlots, _ := processor.getSlots("test-key")
	inFlightSlots <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	requestHandler := &mockRequestHandler{
		ctx:    ctx,
		key:    "test-key",
		bypass: false,
	}

	go func() {
		time.Sleep(time.Millisecond * 100)
		cancel()
	}()

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	s.Error(err)
	s.Contains(err.Error(), "context canceled")
	s.False(requestHandler.executeCalled)
	s.True(requestHandler.onErrorCalled)
	s.Equal("test-key", requestHandler.lastParams.Key)
	s.True(requestHandler.lastParams.RequestBacklogged)
	s.GreaterOrEqual(duration, time.Millisecond*90) // Allow tolerance
	s.LessOrEqual(duration, time.Millisecond*200)
}

func (s *RequestProcessorTestSuite) TestProcessRequest_DryRunRejectWhenSlotsFull() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   2,
		Timeout: time.Second,
	}, true) // Dry run mode
	s.NoError(err)

	// Fill the single in-flight slot
	inFlightSlots, _ := processor.getSlots("test-key")
	inFlightSlots <- struct{}{}

	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	err = processor.ProcessRequest(requestHandler)
	s.NoError(err)
	s.False(requestHandler.executeCalled)
	s.False(requestHandler.onRejectCalled)
	s.True(requestHandler.onRejectInDryRunCalled)
	s.Equal("test-key", requestHandler.lastParams.Key)
	s.True(requestHandler.lastParams.RequestBacklogged)
}

func (s *RequestProcessorTestSuite) TestProcessRequest_DryRunRejectWhenBacklogFull() {
	processor, err := NewRequestProcessor(1, BacklogParams{
		MaxKeys: 0, // Single shared backlog
		Limit:   1, // Backlog limit of 1
		Timeout: time.Second,
	}, true) // Dry run mode
	s.NoError(err)

	// Fill the backlog completely (in-flight + backlog = 1 + 1 = 2 total capacity)
	_, backlogSlots := processor.getSlots("test-key")
	backlogSlots <- struct{}{} // First slot
	backlogSlots <- struct{}{} // Second slot - now backlog is full

	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	err = processor.ProcessRequest(requestHandler)
	s.NoError(err)
	s.False(requestHandler.executeCalled)
	s.False(requestHandler.onRejectCalled)
	s.True(requestHandler.onRejectInDryRunCalled)
	s.Equal("test-key", requestHandler.lastParams.Key)
	s.False(requestHandler.lastParams.RequestBacklogged) // Not backlogged because backlog was full
}

func (s *RequestProcessorTestSuite) TestProcessRequest_ExecuteError() {
	processor, err := NewRequestProcessor(5, BacklogParams{
		MaxKeys: 100,
		Limit:   10,
		Timeout: time.Second,
	}, false)
	s.NoError(err)

	executeErr := errors.New("execute error")
	requestHandler := &mockRequestHandler{
		key:          "test-key",
		bypass:       false,
		executeError: executeErr,
	}

	err = processor.ProcessRequest(requestHandler)
	s.Error(err)
	s.Equal(executeErr, err)
	s.True(requestHandler.executeCalled)
}

// Helper functions and mocks

// mockRequestHandler implements the RequestHandler interface for testing
type mockRequestHandler struct {
	ctx                    context.Context
	key                    string
	bypass                 bool
	keyError               error
	executeError           error
	executeCalled          bool
	onRejectCalled         bool
	onRejectInDryRunCalled bool
	onErrorCalled          bool
	lastParams             Params
	lastError              error
}

func (m *mockRequestHandler) GetContext() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *mockRequestHandler) GetKey() (string, bool, error) {
	return m.key, m.bypass, m.keyError
}

func (m *mockRequestHandler) Execute() error {
	m.executeCalled = true
	return m.executeError
}

func (m *mockRequestHandler) OnReject(params Params) error {
	m.onRejectCalled = true
	m.lastParams = params
	return nil
}

func (m *mockRequestHandler) OnRejectInDryRun(params Params) error {
	m.onRejectInDryRunCalled = true
	m.lastParams = params
	return nil
}

func (m *mockRequestHandler) OnError(params Params, err error) error {
	m.onErrorCalled = true
	m.lastParams = params
	m.lastError = err
	return err
}
