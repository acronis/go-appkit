/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
	"github.com/acronis/go-appkit/retry"
)

type reqInfo struct {
	method             string
	body               []byte
	retryAttemptHeader string
}

type testServerForRetryableRoundTripper struct {
	*httptest.Server
	sync.RWMutex
	reqInfos  []reqInfo
	respCodes []int
}

func (s *testServerForRetryableRoundTripper) ReqInfos() []reqInfo {
	s.RLock()
	defer s.RUnlock()
	res := make([]reqInfo, len(s.reqInfos))
	copy(res, s.reqInfos)
	return res
}

func (s *testServerForRetryableRoundTripper) Reset(respCodes []int) {
	s.Lock()
	defer s.Unlock()
	s.reqInfos = nil
	s.respCodes = respCodes
}

func newTestServerForRetryableRoundTripper() *testServerForRetryableRoundTripper {
	srv := &testServerForRetryableRoundTripper{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var reqBody []byte
		if r.Method != http.MethodGet {
			reqBody, _ = io.ReadAll(r.Body)
		}

		srv.Lock()
		srv.reqInfos = append(srv.reqInfos, reqInfo{
			method:             r.Method,
			body:               reqBody,
			retryAttemptHeader: r.Header.Get(RetryAttemptNumberHeader),
		})
		respCode := http.StatusOK
		if len(srv.respCodes) > 0 {
			respCode = srv.respCodes[len(srv.respCodes)-1]
			srv.respCodes = srv.respCodes[:len(srv.respCodes)-1]
		}
		srv.Unlock()

		rw.WriteHeader(respCode)
		_, _ = rw.Write([]byte("body"))
	}))
	return srv
}

type countingRoundTripper struct {
	delegate http.RoundTripper
	reqsNum  int
}

func (rt *countingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.reqsNum++
	return rt.delegate.RoundTrip(r)
}

type seekOp struct {
	offset int64
	whence int
}

type countableReadSeekCloser struct {
	io.ReadSeeker
	seekOps map[seekOp]int
}

func newCountableReadSeekCloser(rs io.ReadSeeker) *countableReadSeekCloser {
	return &countableReadSeekCloser{rs, make(map[seekOp]int)}
}

func (r *countableReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	r.seekOps[seekOp{offset, whence}]++
	return r.ReadSeeker.Seek(offset, whence)
}

func (r *countableReadSeekCloser) Close() error {
	if closer, ok := r.ReadSeeker.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func TestRetryableRoundTripper_RoundTrip(t *testing.T) {
	testSrv := newTestServerForRetryableRoundTripper()
	defer testSrv.Close()

	reqBodyJSON := []byte(`{"field1":"ultimate_answer_field","field2":42}`)

	genInts := func(val, n int) []int {
		res := make([]int, n)
		for i := 0; i < n; i++ {
			res[i] = val
		}
		return res
	}

	genReqInfos := func(method string, body []byte, n int) []reqInfo {
		res := make([]reqInfo, n)
		for i := 0; i < n; i++ {
			res[i] = reqInfo{method: method, body: body}
			if i > 0 {
				res[i].retryAttemptHeader = strconv.Itoa(i)
			}
		}
		return res
	}

	tests := []struct {
		Name                string
		RetryableRTOpts     RetryableRoundTripperOpts
		ReqMethod           string
		ReqURL              string
		ReqBodyProvider     func() io.Reader
		SrvRespCodes        []int
		WantErr             string
		WantReqsNum         int
		WantFinalRespCode   int
		WantSrvReqInfos     []reqInfo
		WantSeekOps         map[seekOp]int
		WantCloseReqBodyErr string
	}{
		{
			Name:              "GET method, retry on HTTP error",
			RetryableRTOpts:   RetryableRoundTripperOpts{MaxRetryAttempts: 5},
			ReqMethod:         http.MethodGet,
			ReqURL:            testSrv.URL,
			ReqBodyProvider:   func() io.Reader { return nil },
			SrvRespCodes:      genInts(http.StatusServiceUnavailable, 5),
			WantReqsNum:       6,
			WantSrvReqInfos:   genReqInfos(http.MethodGet, nil, 6),
			WantFinalRespCode: http.StatusOK,
		},
		{
			Name: "unlimited retry attempts, but exp backoff retry policy with max attempts, PUT method, retry on HTTP error",
			RetryableRTOpts: RetryableRoundTripperOpts{
				MaxRetryAttempts: UnlimitedRetryAttempts,
				BackoffPolicy:    retry.NewExponentialBackoffPolicy(DefaultExponentialBackoffInitialInterval, 3),
			},
			ReqMethod:         http.MethodPut,
			ReqURL:            testSrv.URL,
			ReqBodyProvider:   func() io.Reader { return bytes.NewReader(reqBodyJSON) },
			SrvRespCodes:      genInts(http.StatusTooManyRequests, 3),
			WantReqsNum:       4,
			WantSrvReqInfos:   genReqInfos(http.MethodPut, reqBodyJSON, 4),
			WantFinalRespCode: http.StatusOK,
		},
		{
			Name: "custom constant backoff retry policy, POST method, retry on HTTP error, fail",
			RetryableRTOpts: RetryableRoundTripperOpts{
				MaxRetryAttempts: 3,
				BackoffPolicy:    retry.NewConstantBackoffPolicy(time.Millisecond*10, 0),
			},
			ReqMethod:         http.MethodPost,
			ReqURL:            testSrv.URL,
			ReqBodyProvider:   func() io.Reader { return bytes.NewReader(reqBodyJSON) },
			SrvRespCodes:      genInts(http.StatusTooManyRequests, 4),
			WantReqsNum:       4,
			WantSrvReqInfos:   genReqInfos(http.MethodPost, reqBodyJSON, 4),
			WantFinalRespCode: http.StatusTooManyRequests,
		},
		{
			Name: "custom constant backoff retry policy, POST method, seeker body",
			RetryableRTOpts: RetryableRoundTripperOpts{
				MaxRetryAttempts: 3,
				BackoffPolicy:    retry.NewConstantBackoffPolicy(time.Millisecond*10, 0),
			},
			ReqMethod:         http.MethodPost,
			ReqURL:            testSrv.URL,
			ReqBodyProvider:   func() io.Reader { return newCountableReadSeekCloser(bytes.NewReader(reqBodyJSON)) },
			SrvRespCodes:      genInts(http.StatusTooManyRequests, 3),
			WantReqsNum:       4,
			WantSrvReqInfos:   genReqInfos(http.MethodPost, reqBodyJSON, 4),
			WantFinalRespCode: http.StatusOK,
			WantSeekOps:       map[seekOp]int{{0, io.SeekCurrent}: 1, {0, io.SeekStart}: 4},
		},
		{
			Name: "custom exp backoff retry policy, POST method, seeker body with no-zero initial offset",
			RetryableRTOpts: RetryableRoundTripperOpts{
				BackoffPolicy: retry.PolicyFunc(func() backoff.BackOff {
					bf := backoff.NewExponentialBackOff()
					bf.Multiplier = 1
					return backoff.WithMaxRetries(bf, 3)
				}),
			},
			ReqMethod: http.MethodPost,
			ReqURL:    testSrv.URL,
			ReqBodyProvider: func() io.Reader {
				r := bytes.NewReader(reqBodyJSON)
				_, _ = r.Seek(10, io.SeekStart)
				return newCountableReadSeekCloser(r)
			},
			SrvRespCodes:      genInts(http.StatusTooManyRequests, 3),
			WantReqsNum:       4,
			WantSrvReqInfos:   genReqInfos(http.MethodPost, reqBodyJSON[10:], 4),
			WantFinalRespCode: http.StatusOK,
			WantSeekOps:       map[seekOp]int{{10, io.SeekStart}: 4, {0, io.SeekCurrent}: 1},
		},
		{
			Name: "custom constant backoff policy, POST method, upload from file directly",
			RetryableRTOpts: RetryableRoundTripperOpts{
				MaxRetryAttempts: 3,
				BackoffPolicy:    retry.NewConstantBackoffPolicy(time.Millisecond*10, 0),
			},
			ReqMethod: http.MethodPost,
			ReqURL:    testSrv.URL,
			ReqBodyProvider: func() io.Reader {
				filePath := path.Join(os.TempDir(), "foobar")
				_ = os.WriteFile(filePath, reqBodyJSON, 0644)
				f, _ := os.Open(filePath)
				return newCountableReadSeekCloser(f)
			},
			SrvRespCodes:        genInts(http.StatusTooManyRequests, 3),
			WantReqsNum:         4,
			WantSrvReqInfos:     genReqInfos(http.MethodPost, reqBodyJSON, 4),
			WantFinalRespCode:   http.StatusOK,
			WantSeekOps:         map[seekOp]int{{0, io.SeekCurrent}: 1, {0, io.SeekStart}: 4},
			WantCloseReqBodyErr: "file already closed",
		},
		{
			Name:            "default retry transport, GET method, url.Error",
			RetryableRTOpts: RetryableRoundTripperOpts{},
			ReqMethod:       http.MethodGet,
			ReqURL:          "foobar",
			ReqBodyProvider: func() io.Reader { return nil },
			WantReqsNum:     1,
			WantSrvReqInfos: make([]reqInfo, 0),
			WantErr:         "unsupported protocol scheme",
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			testSrv.Reset(tt.SrvRespCodes)

			countingRT := &countingRoundTripper{delegate: http.DefaultTransport}
			retryableRT, err := NewRetryableRoundTripperWithOpts(countingRT, tt.RetryableRTOpts)
			require.NoError(t, err)
			httpClient := &http.Client{Transport: retryableRT, Timeout: 60 * time.Second}

			reqBody := tt.ReqBodyProvider()

			req, reqErr := http.NewRequest(tt.ReqMethod, tt.ReqURL, reqBody)
			require.NoError(t, reqErr)

			resp, respErr := httpClient.Do(req)
			if tt.WantErr == "" {
				require.NoError(t, respErr)
				require.Equal(t, tt.WantFinalRespCode, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			} else {
				require.Error(t, respErr)
				require.Contains(t, respErr.Error(), tt.WantErr)
			}
			require.Equal(t, tt.WantReqsNum, countingRT.reqsNum)
			require.Equal(t, tt.WantSrvReqInfos, testSrv.ReqInfos())

			// Check seeks.
			if len(tt.WantSeekOps) > 0 {
				csr, ok := reqBody.(*countableReadSeekCloser)
				require.True(t, ok)
				require.Equal(t, tt.WantSeekOps, csr.seekOps)
			}

			// Close and check error.
			if closer, ok := reqBody.(io.Closer); ok {
				closeErr := closer.Close()
				if tt.WantCloseReqBodyErr == "" {
					require.NoError(t, closeErr)
				} else {
					require.Error(t, closeErr)
					require.Contains(t, closeErr.Error(), tt.WantCloseReqBodyErr)
				}
			}
		})
	}
}

func TestParseRetryAfterFromResponse(t *testing.T) {
	tests := []struct {
		Name                   string
		RetryAfterHeader       string
		WantParsedRetryAfter   time.Duration
		WantParsedRetryAfterOK bool
		CheckParsedRetryAfter  func(t *testing.T, headerRetryAfter string, parsedRetryAfter time.Duration)
	}{
		{
			Name:                   "empty value",
			RetryAfterHeader:       "",
			WantParsedRetryAfterOK: false,
		},
		{
			Name:                   "valid number of seconds",
			RetryAfterHeader:       "600",
			WantParsedRetryAfter:   600 * time.Second,
			WantParsedRetryAfterOK: true,
		},
		{
			Name:                   "valid number of seconds, zero",
			RetryAfterHeader:       "0",
			WantParsedRetryAfter:   0,
			WantParsedRetryAfterOK: true,
		},
		{
			Name:                   "negative number of seconds",
			RetryAfterHeader:       "-1",
			WantParsedRetryAfter:   0,
			WantParsedRetryAfterOK: false,
		},
		{
			Name:                   "malformed date time value",
			RetryAfterHeader:       "Fri, 17 Some Malformed Date GMT",
			WantParsedRetryAfter:   0,
			WantParsedRetryAfterOK: false,
		},
		{
			Name:                   "valid date time value",
			RetryAfterHeader:       "Fri, 17 May 2030 23:00:00 GMT",
			WantParsedRetryAfterOK: true,
			CheckParsedRetryAfter: func(t *testing.T, headerRetryAfter string, parsedRetryAfter time.Duration) {
				parsedTime, _ := time.Parse(time.RFC1123, headerRetryAfter)
				require.InDelta(t, time.Until(parsedTime), parsedRetryAfter, float64(time.Millisecond))
			},
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
			resp.Header.Set("Retry-After", tt.RetryAfterHeader)
			retryAfter, ok := parseRetryAfterFromResponse(resp)
			require.Equal(t, tt.WantParsedRetryAfterOK, ok)
			if tt.CheckParsedRetryAfter != nil {
				tt.CheckParsedRetryAfter(t, tt.RetryAfterHeader, retryAfter)
			} else {
				require.Equal(t, tt.WantParsedRetryAfter, retryAfter)
			}
		})
	}
}

func TestCheckErrorIsTemporary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(nil))
	srv.Config.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			srv.CloseClientConnections()
			return
		}
		time.Sleep(time.Second)
		_, _ = rw.Write([]byte("ok"))
	})

	defer srv.Close()

	tests := []struct {
		Name          string
		ReqMethod     string
		ReqURL        string
		ReqTimeout    time.Duration
		WantTempError bool
		WantErr       string
	}{
		{
			Name:          "invalid url is not temp error",
			ReqMethod:     http.MethodGet,
			ReqURL:        "invalid url",
			ReqTimeout:    time.Second * 3,
			WantTempError: false,
			WantErr:       "unsupported protocol scheme",
		},
		{
			Name:          "request timeout is temp error",
			ReqMethod:     http.MethodGet,
			ReqURL:        srv.URL,
			ReqTimeout:    time.Millisecond * 100,
			WantTempError: true,
			WantErr:       "Client.Timeout exceeded",
		},
		{
			Name:          "EOF is temp error",
			ReqMethod:     http.MethodPost,
			ReqURL:        srv.URL,
			ReqTimeout:    time.Second * 2,
			WantTempError: true,
			WantErr:       "EOF",
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			req, err := http.NewRequest(tt.ReqMethod, tt.ReqURL, nil)
			require.NoError(t, err)
			_, err = (&http.Client{Timeout: tt.ReqTimeout}).Do(req) //nolint:bodyclose
			require.ErrorContains(t, err, tt.WantErr)
			require.Equal(t, tt.WantTempError, CheckErrorIsTemporary(err))
		})
	}
}

func TestRetryableRoundTripper_RoundTrip_Logging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(wr http.ResponseWriter, req *http.Request) {
		wr.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	type ctxKey string
	const ctxKeyLogger ctxKey = "keyLogger"

	internalErr := errors.New("internal error")

	doRequestAndCheckLogs := func(t *testing.T, client *http.Client, req *http.Request, logRecorder *logtest.Recorder) {
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		require.NoError(t, resp.Body.Close())

		require.Len(t, logRecorder.Entries(), 1)
		require.Equal(t, "failed to check if retry is needed, 1 request(s) done", logRecorder.Entries()[0].Text)
		logField, found := logRecorder.Entries()[0].FindField("error")
		require.True(t, found)
		require.Equal(t, internalErr, logField.Any)
	}

	t.Run("logger", func(t *testing.T) {
		logRecorder := logtest.NewRecorder()

		var rt http.RoundTripper
		rt = http.DefaultTransport.(*http.Transport).Clone()
		rt, err := NewRetryableRoundTripperWithOpts(rt, RetryableRoundTripperOpts{
			Logger: logRecorder,
			CheckRetryFunc: func(ctx context.Context, resp *http.Response, roundTripErr error, doneRetryAttempts int) (bool, error) {
				return false, internalErr
			},
		})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)

		doRequestAndCheckLogs(t, &http.Client{Transport: rt}, req, logRecorder)
	})

	t.Run("logger from context", func(t *testing.T) {
		logRecorder := logtest.NewRecorder()

		var rt http.RoundTripper
		rt = http.DefaultTransport.(*http.Transport).Clone()
		rt, err := NewRetryableRoundTripperWithOpts(rt, RetryableRoundTripperOpts{
			LoggerProvider: func(ctx context.Context) log.FieldLogger {
				return ctx.Value(ctxKeyLogger).(log.FieldLogger)
			},
			CheckRetryFunc: func(ctx context.Context, resp *http.Response, roundTripErr error, doneRetryAttempts int) (bool, error) {
				return false, internalErr
			},
		})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		req = req.WithContext(context.WithValue(req.Context(), ctxKeyLogger, logRecorder))

		doRequestAndCheckLogs(t, &http.Client{Transport: rt}, req, logRecorder)
	})
}
