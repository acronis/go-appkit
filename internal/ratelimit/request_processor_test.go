/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// BacklogTestSuite contains tests for backlog functionality
type BacklogTestSuite struct {
	suite.Suite
}

func TestBacklog(t *testing.T) {
	suite.Run(t, new(BacklogTestSuite))
}

func (ts *BacklogTestSuite) TestNewBacklogSlotsProvider() {
	tests := []struct {
		name         string
		backlogLimit int
		maxKeys      int
	}{
		{
			name:         "with max keys",
			backlogLimit: 10,
			maxKeys:      100,
		},
		{
			name:         "zero max keys",
			backlogLimit: 10,
			maxKeys:      0,
		},
		{
			name:         "zero backlog limit",
			backlogLimit: 0,
			maxKeys:      100,
		},
	}

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			provider := newBacklogSlotsProvider(tt.backlogLimit, tt.maxKeys)
			ts.NotNil(provider)

			slots := provider("test-key")
			ts.NotNil(slots)
			ts.Equal(tt.backlogLimit, cap(slots))
		})
	}
}

func (ts *BacklogTestSuite) TestBacklogSlotsProvider_SameKeyReturnsSameSlots() {
	provider := newBacklogSlotsProvider(5, 0) // maxKeys = 0 means single shared backlog
	key := "test-key"

	slots1 := provider(key)
	slots2 := provider(key)

	ts.Equal(slots1, slots2, "same key should return same slots when maxKeys=0")
}

func (ts *BacklogTestSuite) TestBacklogSlotsProvider_DifferentKeysWithLRU() {
	provider := newBacklogSlotsProvider(5, 100)
	key1 := "test-key-1"
	key2 := "test-key-2"

	slots1 := provider(key1)
	slots2 := provider(key2)

	ts.NotEqual(slots1, slots2, "different keys should have different slots")
	ts.Equal(5, cap(slots1))
	ts.Equal(5, cap(slots2))
}

// RequestProcessorTestSuite contains tests for RequestProcessor
type RequestProcessorTestSuite struct {
	suite.Suite
}

func TestRequestProcessor(t *testing.T) {
	suite.Run(t, new(RequestProcessorTestSuite))
}

func (ts *RequestProcessorTestSuite) TestNewRequestProcessor() {
	tests := []struct {
		name           string
		limiter        Limiter
		backlogParams  BacklogParams
		wantErr        bool
		expectedErrMsg string
	}{
		{
			name:    "valid parameters",
			limiter: &mockLimiter{},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: time.Second,
			},
			wantErr: false,
		},
		{
			name:    "zero backlog limit",
			limiter: &mockLimiter{},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			wantErr: false,
		},
		{
			name:    "negative backlog limit",
			limiter: &mockLimiter{},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   -1,
				Timeout: time.Second,
			},
			wantErr:        true,
			expectedErrMsg: "backlog limit should not be negative",
		},
		{
			name:    "negative max keys",
			limiter: &mockLimiter{},
			backlogParams: BacklogParams{
				MaxKeys: -1,
				Limit:   10,
				Timeout: time.Second,
			},
			wantErr:        true,
			expectedErrMsg: "max keys for backlog should not be negative",
		},
		{
			name:    "zero timeout uses default",
			limiter: &mockLimiter{},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   10,
				Timeout: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			processor, err := NewRequestProcessor(tt.limiter, tt.backlogParams)
			if tt.wantErr {
				ts.Error(err)
				ts.Contains(err.Error(), tt.expectedErrMsg)
				ts.Nil(processor)
			} else {
				ts.NoError(err)
				ts.NotNil(processor)
				ts.Equal(tt.limiter, processor.limiter)
				if tt.backlogParams.Limit > 0 {
					ts.NotNil(processor.getBacklogSlots)
				} else {
					ts.Nil(processor.getBacklogSlots)
				}
				expectedTimeout := tt.backlogParams.Timeout
				if expectedTimeout == 0 {
					expectedTimeout = DefaultRateLimitBacklogTimeout
				}
				ts.Equal(expectedTimeout, processor.backlogTimeout)
			}
		})
	}
}

func (ts *RequestProcessorTestSuite) TestProcessRequest() {
	tests := []struct {
		name                string
		limiter             Limiter
		backlogParams       BacklogParams
		requestHandler      *mockRequestHandler
		expectedError       string
		expectedExecuteCall bool
	}{
		{
			name:    "bypass rate limiting",
			limiter: &mockLimiter{allowResult: true},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: true,
			},
			expectedExecuteCall: true,
		},
		{
			name:    "allow request",
			limiter: &mockLimiter{allowResult: true},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: false,
			},
			expectedExecuteCall: true,
		},
		{
			name:    "reject request without backlog",
			limiter: &mockLimiter{allowResult: false, retryAfter: time.Second},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: false,
			},
			expectedExecuteCall: false,
		},
		{
			name:    "get key error",
			limiter: &mockLimiter{allowResult: true},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			requestHandler: &mockRequestHandler{
				keyError: errors.New("key error"),
			},
			expectedError:       "get key for rate limit: key error",
			expectedExecuteCall: false,
		},
		{
			name:    "limiter error",
			limiter: &mockLimiter{allowError: errors.New("limiter error")},
			backlogParams: BacklogParams{
				MaxKeys: 100,
				Limit:   0,
				Timeout: time.Second,
			},
			requestHandler: &mockRequestHandler{
				key:    "test-key",
				bypass: false,
			},
			expectedError:       "rate limit: limiter error",
			expectedExecuteCall: false,
		},
	}

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			processor, err := NewRequestProcessor(tt.limiter, tt.backlogParams)
			ts.NoError(err)

			err = processor.ProcessRequest(tt.requestHandler)

			if tt.expectedError != "" {
				ts.Error(err)
				ts.Contains(err.Error(), tt.expectedError)
			} else {
				ts.NoError(err)
			}

			ts.Equal(tt.expectedExecuteCall, tt.requestHandler.executeCalled)
		})
	}
}

func (ts *RequestProcessorTestSuite) TestProcessRequest_WithBacklog() {
	limiter := &mockLimiter{
		allowResults: []bool{false, true}, // First call fails, second succeeds
		retryAfter:   time.Millisecond * 50,
	}
	backlogParams := BacklogParams{
		MaxKeys: 100,
		Limit:   1,
		Timeout: time.Second,
	}
	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	processor, err := NewRequestProcessor(limiter, backlogParams)
	ts.NoError(err)

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	ts.NoError(err)
	ts.True(requestHandler.executeCalled)
	ts.GreaterOrEqual(duration, time.Millisecond*40) // Allow some tolerance
}

func (ts *RequestProcessorTestSuite) TestProcessRequest_BacklogTimeout() {
	limiter := &mockLimiter{
		allowResult: false,
		retryAfter:  time.Second,
	}
	backlogParams := BacklogParams{
		MaxKeys: 100,
		Limit:   1,
		Timeout: time.Millisecond * 100,
	}
	requestHandler := &mockRequestHandler{
		key:    "test-key",
		bypass: false,
	}

	processor, err := NewRequestProcessor(limiter, backlogParams)
	ts.NoError(err)

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	ts.NoError(err)
	ts.False(requestHandler.executeCalled)
	ts.True(requestHandler.onRejectCalled)
	ts.GreaterOrEqual(duration, time.Millisecond*90) // Allow tolerance
	ts.LessOrEqual(duration, time.Millisecond*200)
}

func (ts *RequestProcessorTestSuite) TestProcessRequest_ContextCancellation() {
	limiter := &mockLimiter{
		allowResult: false,
		retryAfter:  time.Second,
	}
	backlogParams := BacklogParams{
		MaxKeys: 100,
		Limit:   1,
		Timeout: time.Second * 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	requestHandler := &mockRequestHandler{
		ctx:    ctx,
		key:    "test-key",
		bypass: false,
	}

	processor, err := NewRequestProcessor(limiter, backlogParams)
	ts.NoError(err)

	go func() {
		time.Sleep(time.Millisecond * 100)
		cancel()
	}()

	start := time.Now()
	err = processor.ProcessRequest(requestHandler)
	duration := time.Since(start)

	ts.Error(err)
	ts.Contains(err.Error(), "context canceled")
	ts.False(requestHandler.executeCalled)
	ts.True(requestHandler.onErrorCalled)
	ts.GreaterOrEqual(duration, time.Millisecond*90) // Allow tolerance
	ts.LessOrEqual(duration, time.Millisecond*200)
}

// Helper functions and mocks

// mockLimiter implements the Limiter interface for testing
type mockLimiter struct {
	allowResult  bool
	allowResults []bool
	allowIndex   int
	allowError   error
	retryAfter   time.Duration
}

func (m *mockLimiter) Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	if m.allowError != nil {
		return false, 0, m.allowError
	}

	if m.allowResults != nil {
		if m.allowIndex < len(m.allowResults) {
			result := m.allowResults[m.allowIndex]
			m.allowIndex++
			return result, m.retryAfter, nil
		}
		return false, m.retryAfter, nil
	}

	return m.allowResult, m.retryAfter, nil
}

// mockRequestHandler implements the RequestHandler interface for testing
type mockRequestHandler struct {
	ctx            context.Context
	key            string
	bypass         bool
	keyError       error
	executeError   error
	executeCalled  bool
	onRejectCalled bool
	onErrorCalled  bool
	lastParams     Params
	lastError      error
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

func (m *mockRequestHandler) OnError(params Params, err error) error {
	m.onErrorCalled = true
	m.lastParams = params
	m.lastError = err
	return err
}
