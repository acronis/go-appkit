/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

type responseInfo struct {
	resp       *http.Response
	err        error
	startedAt  time.Time
	finishedAt time.Time
}

func doGet(c *http.Client, url string) responseInfo {
	startedAt := time.Now()
	resp, err := c.Get(url)
	finishedAt := time.Now()
	if err == nil {
		_ = resp.Body.Close()
	}
	return responseInfo{resp, err, startedAt, finishedAt}
}

func makeTestServerForRateLimitingRoundTripper(adaptiveRateLimitHeader string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if adaptiveRateLimitHeader != "" {
			if rl := r.URL.Query().Get("rateLimit"); rl != "" {
				rw.Header().Set(adaptiveRateLimitHeader, rl)
			}
		}
		_, _ = rw.Write([]byte("ok"))
	}))
}

func TestNewRateLimitingRoundTripper(t *testing.T) {
	tests := []struct {
		Name       string
		RateLimit  int
		Opts       RateLimitingRoundTripperOpts
		WantErrMsg string
	}{
		{
			Name:       "rate limit is negative",
			RateLimit:  -1,
			WantErrMsg: "rate limit must be positive",
		},
		{
			Name:       "rate limit is zero",
			RateLimit:  0,
			WantErrMsg: "rate limit must be positive",
		},
		{
			Name:       "burst is negative",
			RateLimit:  1,
			Opts:       RateLimitingRoundTripperOpts{Burst: -1},
			WantErrMsg: "burst must be positive",
		},
		{
			Name:       "slack percent < 0",
			RateLimit:  1,
			Opts:       RateLimitingRoundTripperOpts{Adaptation: RateLimitingRoundTripperAdaptation{SlackPercent: -1}},
			WantErrMsg: "slack percent must be in range [0..100]",
		},
		{
			Name:       "slack percent > 100",
			RateLimit:  1,
			Opts:       RateLimitingRoundTripperOpts{Adaptation: RateLimitingRoundTripperAdaptation{SlackPercent: 101}},
			WantErrMsg: "slack percent must be in range [0..100]",
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			_, err := NewRateLimitingRoundTripperWithOpts(http.DefaultTransport, tt.RateLimit, tt.Opts)
			require.EqualError(t, err, tt.WantErrMsg)
		})
	}
}

func TestRateLimitingRoundTripper_RoundTrip(t *testing.T) {
	const allowedTimeDeviation = time.Millisecond * 100

	server := makeTestServerForRateLimitingRoundTripper("")
	defer server.Close()

	makeClient := func(rateLimit int, waitTimeout time.Duration) *http.Client {
		opts := RateLimitingRoundTripperOpts{WaitTimeout: waitTimeout}
		tr, err := NewRateLimitingRoundTripperWithOpts(http.DefaultTransport, rateLimit, opts)
		require.NoError(t, err)
		return &http.Client{Transport: tr}
	}

	t.Run("waiting rate limit is timed out for the 2nd request", func(t *testing.T) {
		client := makeClient(1, time.Millisecond*500)
		var respInfo responseInfo

		// The first request should be completed immediately.
		respInfo = doGet(client, server.URL)
		require.NoError(t, respInfo.err, "the 1st request should be finished without error")
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)

		// The second request should be throttled and error should be received (waiting timeout is not enough).
		respInfo = doGet(client, server.URL)
		var waitErr *RateLimitingWaitError
		require.ErrorAs(t, respInfo.err, &waitErr,
			"the 2nd request should be finished with error since wait timeout for rate limiting is not enough")
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation,
			"error about too many requests should be returned immediately")
	})

	t.Run("waiting rate limit is timed out for the Nth request", func(t *testing.T) {
		const rateLimit = 4
		const reqsCount = rateLimit + 1

		client := makeClient(rateLimit, time.Second-time.Second/rateLimit+allowedTimeDeviation)
		errsCh := make(chan error, reqsCount)

		startedAt := time.Now()
		var wg sync.WaitGroup
		wg.Add(reqsCount)
		for i := 0; i < reqsCount; i++ {
			go func() {
				defer wg.Done()
				respInfo := doGet(client, server.URL)
				errsCh <- respInfo.err
			}()
		}
		wg.Wait()
		finishedAt := time.Now()

		close(errsCh)

		require.WithinDuration(t, startedAt.Add(time.Second-time.Second/rateLimit), finishedAt, allowedTimeDeviation)

		succeededCount := 0
		var errs []error
		for err := range errsCh {
			if err != nil {
				errs = append(errs, err)
				continue
			}
			succeededCount++
		}
		require.Equal(t, rateLimit, succeededCount)
		require.Equal(t, 1, len(errs), "one request should be finished with error")
		var waitErr *RateLimitingWaitError
		require.ErrorAs(t, errs[0], &waitErr)
	})

	t.Run("the 2nd request is throttled", func(t *testing.T) {
		client := makeClient(1, time.Second*2)
		var respInfo responseInfo

		// The first request should be completed immediately.
		respInfo = doGet(client, server.URL)
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation,
			"the 1st request should be finished immediately")

		// The second request should be throttled.
		respInfo = doGet(client, server.URL)
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt.Add(time.Second), respInfo.finishedAt, allowedTimeDeviation,
			"the 2nd request should be throttled")
	})

	t.Run("requests are throttled", func(t *testing.T) {
		const rateLimit = 4

		client := makeClient(rateLimit, time.Second*2)
		ch := make(chan responseInfo, rateLimit)

		batchStartedAt := time.Now()
		var wg sync.WaitGroup
		wg.Add(rateLimit)
		for i := 0; i < rateLimit; i++ {
			go func() {
				defer wg.Done()
				ch <- doGet(client, server.URL)
			}()
		}
		wg.Wait()
		batchFinishedAt := time.Now()

		close(ch)

		require.WithinDuration(t, batchStartedAt.Add(time.Second-time.Second/rateLimit), batchFinishedAt, allowedTimeDeviation)

		for i := 0; i < rateLimit; i++ {
			ri := <-ch
			require.NoError(t, ri.err)
			require.WithinDuration(t, ri.startedAt.Add(time.Second/rateLimit*time.Duration(i)), ri.finishedAt, allowedTimeDeviation)
		}
	})
}

func TestRateLimitingRoundTripper_RoundTrip_Adaptation(t *testing.T) {
	const allowedTimeDeviation = time.Millisecond * 100
	const adaptiveRateLimitHeader = "X-Rate-Limit"

	server := makeTestServerForRateLimitingRoundTripper(adaptiveRateLimitHeader)
	defer server.Close()

	makeAdaptiveClient := func(rateLimit int, respSlackPercent int) (*http.Client, *RateLimitingRoundTripper) {
		tr, err := NewRateLimitingRoundTripperWithOpts(http.DefaultTransport, rateLimit, RateLimitingRoundTripperOpts{
			Adaptation: RateLimitingRoundTripperAdaptation{
				ResponseHeaderName: adaptiveRateLimitHeader,
				SlackPercent:       respSlackPercent,
			},
		})
		require.NoError(t, err)
		return &http.Client{Transport: tr}, tr
	}

	t.Run("requests is throttled by limit from response's header", func(t *testing.T) {
		client, transport := makeAdaptiveClient(5, 0)

		for i := 0; i < 5; i++ {
			url := server.URL
			if i == 4 {
				url += "?rateLimit=1"
			}
			respInfo := doGet(client, url)
			require.NoError(t, respInfo.err)
			require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
			if i == 0 {
				require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
			} else {
				require.WithinDuration(t, respInfo.startedAt.Add(time.Second/5), respInfo.finishedAt, allowedTimeDeviation)
			}
		}
		require.Equal(t, rate.Limit(1), transport.rateLimiter.Limit())
		require.Equal(t, 5, transport.RateLimit)

		respInfo := doGet(client, server.URL)
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt.Add(time.Second), respInfo.finishedAt, allowedTimeDeviation)
	})

	t.Run("rate limit should not be greater than initial value", func(t *testing.T) {
		client, transport := makeAdaptiveClient(10, 0)
		respInfo := doGet(client, server.URL+"?rateLimit=20")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(10), transport.rateLimiter.Limit())
		require.Equal(t, 10, transport.RateLimit)
	})

	t.Run("use non zero slack percent", func(t *testing.T) {
		client, transport := makeAdaptiveClient(10, 20)
		respInfo := doGet(client, server.URL+"?rateLimit=10")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(8), transport.rateLimiter.Limit())
		require.Equal(t, 10, transport.RateLimit)
	})

	t.Run("invalid rate limit values should not be used", func(t *testing.T) {
		var respInfo responseInfo
		client, transport := makeAdaptiveClient(100, 0)

		respInfo = doGet(client, server.URL+"?rateLimit=foobar")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(100), transport.rateLimiter.Limit())
		require.Equal(t, 100, transport.RateLimit)

		respInfo = doGet(client, server.URL+"?rateLimit=-1")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(100), transport.rateLimiter.Limit())
		require.Equal(t, 100, transport.RateLimit)

		respInfo = doGet(client, server.URL+"?rateLimit=1.1")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(100), transport.rateLimiter.Limit())
		require.Equal(t, 100, transport.RateLimit)
	})

	t.Run("continue send request even if rateLimit is 0 in response's header", func(t *testing.T) {
		client, transport := makeAdaptiveClient(10, 0)
		respInfo := doGet(client, server.URL+"?rateLimit=0")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(1), transport.rateLimiter.Limit())
		require.Equal(t, 10, transport.RateLimit)
	})

	t.Run("rateLimit should be reverted", func(t *testing.T) {
		var respInfo responseInfo
		client, transport := makeAdaptiveClient(10, 0)

		respInfo = doGet(client, server.URL+"?rateLimit=5")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt, respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(5), transport.rateLimiter.Limit())
		require.Equal(t, 10, transport.RateLimit)

		respInfo = doGet(client, server.URL+"?rateLimit=100")
		require.NoError(t, respInfo.err)
		require.Equal(t, http.StatusOK, respInfo.resp.StatusCode)
		require.WithinDuration(t, respInfo.startedAt.Add(time.Second/5), respInfo.finishedAt, allowedTimeDeviation)
		require.Equal(t, rate.Limit(10), transport.rateLimiter.Limit())
		require.Equal(t, 10, transport.RateLimit)
	})
}
