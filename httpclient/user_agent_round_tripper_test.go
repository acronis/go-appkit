/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserAgentRoundTripper_RoundTrip(t *testing.T) {
	const (
		reqUserAgentHeader  = "User-Agent"
		respUserAgentHeader = "X-User-Agent"
	)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(respUserAgentHeader, r.Header.Get(reqUserAgentHeader))
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tests := []struct {
		name             string
		reqUserAgent     string
		rtUserAgent      string
		rtUpdateStrategy UserAgentUpdateStrategy
		wantUserAgent    string
	}{
		{
			name:             "set if empty",
			reqUserAgent:     "",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategySetIfEmpty,
			wantUserAgent:    "my-service-rt/1.0",
		},
		{
			name:             "set if empty, existing",
			reqUserAgent:     "my-service-req/0.1",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategySetIfEmpty,
			wantUserAgent:    "my-service-req/0.1",
		},
		{
			name:             "append, empty",
			reqUserAgent:     "",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategyAppend,
			wantUserAgent:    "my-service-rt/1.0",
		},
		{
			name:             "append, existing",
			reqUserAgent:     "my-service-req/0.1",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategyAppend,
			wantUserAgent:    "my-service-req/0.1 my-service-rt/1.0",
		},
		{
			name:             "prepend, empty",
			reqUserAgent:     "",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategyPrepend,
			wantUserAgent:    "my-service-rt/1.0",
		},
		{
			name:             "prepend, existing",
			reqUserAgent:     "my-service-req/0.1",
			rtUserAgent:      "my-service-rt/1.0",
			rtUpdateStrategy: UserAgentUpdateStrategyPrepend,
			wantUserAgent:    "my-service-rt/1.0 my-service-req/0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set(reqUserAgentHeader, tt.reqUserAgent)
			rt := NewUserAgentRoundTripperWithOpts(http.DefaultTransport, tt.rtUserAgent, UserAgentRoundTripperOpts{
				UpdateStrategy: tt.rtUpdateStrategy,
			})
			client := http.Client{Transport: rt}
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, tt.wantUserAgent, resp.Header.Get(respUserAgentHeader))
		})
	}
}
