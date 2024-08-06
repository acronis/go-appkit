/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestRateLimitHandler_ServeHTTP(t *testing.T) {
	const errDomain = "MyService"

	makeReqAndRespRec := func() (*http.Request, *httptest.ResponseRecorder) {
		return httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()
	}

	makeNext := func() (next http.HandlerFunc, servedCount *atomic.Int32) {
		servedCount = atomic.NewInt32(0)
		next = func(rw http.ResponseWriter, r *http.Request) {
			servedCount.Inc()
			rw.WriteHeader(http.StatusOK)
		}
		return
	}

	getRetryAfterFromResp := func(respRec *httptest.ResponseRecorder) (time.Duration, error) {
		retryAfterHeader := respRec.Header().Get("Retry-After")
		if retryAfterHeader == "" {
			return 0, fmt.Errorf("header Retry-After is empty")
		}
		retryAfterSecs, err := strconv.Atoi(retryAfterHeader)
		if err != nil {
			return 0, fmt.Errorf("converting header Retry-After to int: %w", err)
		}
		return time.Second * time.Duration(retryAfterSecs), nil
	}

	sendReqAndCheckCode := func(t *testing.T, handler http.Handler, wantCode int, headers http.Header) (retryAfter time.Duration) {
		t.Helper()
		req, respRec := makeReqAndRespRec()
		req.Header = headers
		handler.ServeHTTP(respRec, req)
		require.Equal(t, wantCode, respRec.Code)
		if wantCode == http.StatusServiceUnavailable || wantCode == http.StatusTooManyRequests {
			var err error
			retryAfter, err = getRetryAfterFromResp(respRec)
			require.NoError(t, err)
		}
		return
	}

	t.Run("leaky bucket, maxRate=1r/s, maxBurst=0, no key", func(t *testing.T) {
		next, nextServedCount := makeNext()
		handler := RateLimit(Rate{1, time.Second}, errDomain)(next)
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		retryAfter := sendReqAndCheckCode(t, handler, http.StatusServiceUnavailable, nil)
		time.Sleep(retryAfter)
		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		require.Equal(t, 2, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=10r/s, maxBurst=10, no key", func(t *testing.T) {
		rate := Rate{10, time.Second}
		const (
			maxBurst          = 10
			concurrentReqsNum = 20
			serialReqsNum     = 10
		)

		emissionInterval := rate.Duration / time.Duration(rate.Count)
		wantRetryAfter := time.Second * time.Duration(math.Ceil(emissionInterval.Seconds()))

		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(rate, errDomain, RateLimitOpts{MaxBurst: maxBurst, GetRetryAfter: GetRetryAfterEstimatedTime})(next)

		sendNReqsConcurrentlyAndCheck := func(n int) {
			var okCount, tooManyReqsCount, unexpectedCodeReqsCount, wrongRetryAfterReqsCount, getRetryAfterErrsCount atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					req, respRec := makeReqAndRespRec()
					handler.ServeHTTP(respRec, req)
					switch respRec.Code {
					case http.StatusOK:
						okCount.Inc()
					case http.StatusServiceUnavailable:
						tooManyReqsCount.Inc()
						retryAfter, err := getRetryAfterFromResp(respRec)
						if err != nil {
							getRetryAfterErrsCount.Inc()
							return
						}
						if retryAfter != wantRetryAfter {
							wrongRetryAfterReqsCount.Inc()
						}
					default:
						unexpectedCodeReqsCount.Inc()
					}
				}()
			}
			wg.Wait()

			require.Equal(t, 0, int(getRetryAfterErrsCount.Load()))
			require.Equal(t, maxBurst+1, int(okCount.Load()))
			require.Equal(t, n-maxBurst-1, int(tooManyReqsCount.Load()))
			require.Equal(t, 0, int(unexpectedCodeReqsCount.Load()))
			require.Equal(t, 0, int(wrongRetryAfterReqsCount.Load()))
		}

		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)

		for i := 0; i < serialReqsNum; i++ {
			time.Sleep(emissionInterval)
			sendReqAndCheckCode(t, handler, http.StatusOK, nil)
			retryAfter := sendReqAndCheckCode(t, handler, http.StatusServiceUnavailable, nil)
			require.Equal(t, wantRetryAfter, retryAfter)
		}
		time.Sleep(emissionInterval * (maxBurst + 1)) // Wait until burst slots are free plus emission interval.

		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)

		require.Equal(t, serialReqsNum+(maxBurst+1)*2, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=1r/s, maxBurst=0, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{
			GetKey:             makeRateLimitGetKeyByHeader(headerClientID),
			GetRetryAfter:      GetRetryAfterEstimatedTime,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		client1Headers := http.Header{}
		client1Headers.Set(headerClientID, "client-1")
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, client1Headers)

		client2Headers := http.Header{}
		client2Headers.Set(headerClientID, "client-2")
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, client2Headers)

		retryAfter := sendReqAndCheckCode(t, handler, http.StatusTooManyRequests, client1Headers)
		time.Sleep(retryAfter)
		sendReqAndCheckCode(t, handler, http.StatusOK, client1Headers)

		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)

		require.Equal(t, 5, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=1r/s, maxBurst=0, by key, no bypass empty key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{
			GetKey: func(r *http.Request) (key string, bypass bool, err error) {
				return r.Header.Get(headerClientID), false, nil
			},
			ResponseStatusCode: http.StatusTooManyRequests,
			GetRetryAfter:      GetRetryAfterEstimatedTime,
		})(next)

		client1Headers := http.Header{}
		client1Headers.Set(headerClientID, "client-1")
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, client1Headers)
		retryAfter := sendReqAndCheckCode(t, handler, http.StatusTooManyRequests, client1Headers)
		time.Sleep(retryAfter)
		sendReqAndCheckCode(t, handler, http.StatusOK, client1Headers)

		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		retryAfter = sendReqAndCheckCode(t, handler, http.StatusTooManyRequests, nil)
		time.Sleep(retryAfter)
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)

		require.Equal(t, 4, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=10r/s, maxBurst=10, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		rate := Rate{10, time.Second}
		const (
			maxBurst          = 10
			concurrentReqsNum = 20
			serialReqsNum     = 10
			clientsNum        = 5
		)

		emissionInterval := rate.Duration / time.Duration(rate.Count)
		wantRetryAfter := time.Second * time.Duration(math.Ceil(emissionInterval.Seconds()))

		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(rate, errDomain, RateLimitOpts{
			MaxBurst:           maxBurst,
			GetKey:             makeRateLimitGetKeyByHeader(headerClientID),
			GetRetryAfter:      GetRetryAfterEstimatedTime,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		sendNReqsConcurrentlyAndCheck := func(n int) {
			respStats := make([]struct {
				okCount                  atomic.Int32
				tooManyReqsCount         atomic.Int32
				unexpectedCodeReqsCount  atomic.Int32
				wrongRetryAfterReqsCount atomic.Int32
				getRetryAfterErrsCount   atomic.Int32
			}, clientsNum)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				for j := 0; j < clientsNum; j++ {
					wg.Add(1)
					go func(clientIndex int) {
						defer wg.Done()
						req, respRec := makeReqAndRespRec()
						req.Header.Set(headerClientID, fmt.Sprintf("client-%d", clientIndex+1))
						handler.ServeHTTP(respRec, req)
						switch respRec.Code {
						case http.StatusOK:
							respStats[clientIndex].okCount.Inc()
						case http.StatusTooManyRequests:
							respStats[clientIndex].tooManyReqsCount.Inc()
							retryAfter, err := getRetryAfterFromResp(respRec)
							if err != nil {
								respStats[clientIndex].getRetryAfterErrsCount.Inc()
								return
							}
							if retryAfter != wantRetryAfter {
								respStats[clientIndex].wrongRetryAfterReqsCount.Inc()
							}
						default:
							respStats[clientIndex].unexpectedCodeReqsCount.Inc()
						}
					}(j)
				}
			}
			wg.Wait()

			for i := 0; i < clientsNum; i++ {
				require.Equal(t, 0, int(respStats[i].getRetryAfterErrsCount.Load()))
				require.Equal(t, maxBurst+1, int(respStats[i].okCount.Load()))
				require.Equal(t, n-maxBurst-1, int(respStats[i].tooManyReqsCount.Load()))
				require.Equal(t, 0, int(respStats[i].unexpectedCodeReqsCount.Load()))
				require.Equal(t, 0, int(respStats[i].wrongRetryAfterReqsCount.Load()))
			}
		}

		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)

		var okCount, tooManyReqsCount, unexpectedCodeReqsCount, wrongRetryAfterReqsCount, getRetryAfterErrsCount atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < clientsNum; i++ {
			wg.Add(1)
			go func(clientIndex int) {
				defer wg.Done()
				for j := 0; j < serialReqsNum; j++ {
					time.Sleep(emissionInterval)
					req, respRec := makeReqAndRespRec()
					req.Header.Set(headerClientID, fmt.Sprintf("client-%d", clientIndex+1))
					handler.ServeHTTP(respRec, req)
					if respRec.Code == http.StatusOK {
						okCount.Inc()
					} else {
						unexpectedCodeReqsCount.Inc()
					}

					req, respRec = makeReqAndRespRec()
					req.Header.Set(headerClientID, fmt.Sprintf("client-%d", clientIndex+1))
					handler.ServeHTTP(respRec, req)
					if respRec.Code == http.StatusTooManyRequests {
						tooManyReqsCount.Inc()
					} else {
						unexpectedCodeReqsCount.Inc()
					}
					retryAfter, err := getRetryAfterFromResp(respRec)
					if err != nil {
						getRetryAfterErrsCount.Inc()
						return
					}
					if retryAfter != wantRetryAfter {
						wrongRetryAfterReqsCount.Inc()
					}
				}
			}(i)
		}
		wg.Wait()
		require.Equal(t, clientsNum*serialReqsNum, int(okCount.Load()))
		require.Equal(t, clientsNum*serialReqsNum, int(tooManyReqsCount.Load()))
		require.Equal(t, 0, int(unexpectedCodeReqsCount.Load()))
		require.Equal(t, 0, int(wrongRetryAfterReqsCount.Load()))

		time.Sleep(emissionInterval * (maxBurst + 1)) // Wait until burst slots are free plus emission interval.

		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)

		require.Equal(t, clientsNum*(serialReqsNum+(maxBurst+1)*2), int(nextServedCount.Load()))
	})

	t.Run("sliding window, maxRate=1r/s, no key", func(t *testing.T) {
		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{
			Alg:           RateLimitAlgSlidingWindow,
			GetRetryAfter: GetRetryAfterEstimatedTime,
		})(next)
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		retryAfter := sendReqAndCheckCode(t, handler, http.StatusServiceUnavailable, nil)
		time.Sleep(retryAfter)
		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		require.Equal(t, 2, int(nextServedCount.Load()))
	})

	t.Run("sliding window, maxRate=10r/s, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		const concurrentReqsNum = 5
		const clientsNum = 5

		rate := Rate{2, time.Second}

		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(rate, errDomain, RateLimitOpts{
			Alg:                RateLimitAlgSlidingWindow,
			GetKey:             makeRateLimitGetKeyByHeader(headerClientID),
			GetRetryAfter:      GetRetryAfterEstimatedTime,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		sendNReqsConcurrentlyAndCheck := func(n int) {
			respStats := make([]struct {
				okCount                 atomic.Int32
				tooManyReqsCount        atomic.Int32
				unexpectedCodeReqsCount atomic.Int32
				getRetryAfterErrsCount  atomic.Int32
			}, clientsNum)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				for j := 0; j < clientsNum; j++ {
					wg.Add(1)
					go func(clientIndex int) {
						defer wg.Done()
						req, respRec := makeReqAndRespRec()
						req.Header.Set(headerClientID, fmt.Sprintf("client-%d", clientIndex+1))
						handler.ServeHTTP(respRec, req)
						switch respRec.Code {
						case http.StatusOK:
							respStats[clientIndex].okCount.Inc()
						case http.StatusTooManyRequests:
							respStats[clientIndex].tooManyReqsCount.Inc()
							_, err := getRetryAfterFromResp(respRec)
							if err != nil {
								respStats[clientIndex].getRetryAfterErrsCount.Inc()
								return
							}
						default:
							respStats[clientIndex].unexpectedCodeReqsCount.Inc()
						}
					}(j)
				}
			}
			wg.Wait()

			for i := 0; i < clientsNum; i++ {
				require.Equal(t, 0, int(respStats[i].getRetryAfterErrsCount.Load()))
				require.Equal(t, rate.Count, int(respStats[i].okCount.Load()))
				require.Equal(t, n-rate.Count, int(respStats[i].tooManyReqsCount.Load()))
				require.Equal(t, 0, int(respStats[i].unexpectedCodeReqsCount.Load()))
			}
		}

		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)
		time.Sleep(rate.Duration * 2)
		sendNReqsConcurrentlyAndCheck(concurrentReqsNum)
		require.Equal(t, clientsNum*rate.Count*2, int(nextServedCount.Load()))
	})

	t.Run("RetryAfter custom", func(t *testing.T) {
		next, _ := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{
			GetRetryAfter: func(r *http.Request, estimatedTime time.Duration) time.Duration {
				return estimatedTime * 3
			},
		})(next)
		_ = sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		retryAfter := sendReqAndCheckCode(t, handler, http.StatusServiceUnavailable, nil)
		require.Equal(t, time.Second*3, retryAfter)
	})

	t.Run("leaky bucket, maxRate=1r/s, maxBurst=0, backlogLimit=1, no key", func(t *testing.T) {
		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{BacklogLimit: 1})(next)
		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		startTime := time.Now()
		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		require.WithinDuration(t, startTime.Add(time.Second), time.Now(), time.Millisecond*500)
		require.Equal(t, 2, int(nextServedCount.Load()))

		time.Sleep(time.Second)

		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		codes := make(chan int)
		for i := 0; i < 2; i++ {
			go func() {
				req, respRec := makeReqAndRespRec()
				handler.ServeHTTP(respRec, req)
				codes <- respRec.Code
			}()
		}
		require.Equal(t, http.StatusServiceUnavailable, <-codes)
		require.Equal(t, http.StatusOK, <-codes)
		require.Equal(t, 4, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=1r/m, maxBurst=0, backlogLimit=1, backlogTimeout=1s, no key", func(t *testing.T) {
		next, nextServedCount := makeNext()
		rateLimitOpts := RateLimitOpts{BacklogLimit: 1, BacklogTimeout: time.Second, GetRetryAfter: GetRetryAfterEstimatedTime}
		handler := RateLimitWithOpts(Rate{1, time.Minute}, errDomain, rateLimitOpts)(next)
		sendReqAndCheckCode(t, handler, http.StatusOK, nil)
		startTime := time.Now()
		sendReqAndCheckCode(t, handler, http.StatusServiceUnavailable, nil)
		require.WithinDuration(t, startTime.Add(time.Second), time.Now(), time.Millisecond*500)
		require.Equal(t, 1, int(nextServedCount.Load()))
	})

	t.Run("leaky bucket, maxRate=1r/s, maxBurst=0, backlogLimit=1, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		next, nextServedCount := makeNext()
		handler := RateLimitWithOpts(Rate{1, time.Second}, errDomain, RateLimitOpts{
			GetKey:             makeRateLimitGetKeyByHeader(headerClientID),
			BacklogLimit:       1,
			GetRetryAfter:      GetRetryAfterEstimatedTime,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		client1Headers := http.Header{}
		client1Headers.Set(headerClientID, "client-1")
		sendReqAndCheckCode(t, handler, http.StatusOK, client1Headers)

		client2Headers := http.Header{}
		client2Headers.Set(headerClientID, "client-2")
		sendReqAndCheckCode(t, handler, http.StatusOK, client2Headers)

		startTime := time.Now()
		codes := make(chan int)
		for i := 0; i < 4; i++ {
			go func(i int) {
				req, respRec := makeReqAndRespRec()
				clientID := "client-1"
				if i%2 != 0 {
					clientID = "client-2"
				}
				req.Header.Set(headerClientID, clientID)
				handler.ServeHTTP(respRec, req)
				codes <- respRec.Code
			}(i)
		}
		require.Equal(t, http.StatusTooManyRequests, <-codes)
		require.Equal(t, http.StatusTooManyRequests, <-codes)
		require.Equal(t, http.StatusOK, <-codes)
		require.Equal(t, http.StatusOK, <-codes)
		require.WithinDuration(t, startTime.Add(time.Second), time.Now(), time.Second)

		require.Equal(t, 4, int(nextServedCount.Load()))
	})
}

//nolint:unparam
func makeRateLimitGetKeyByHeader(headerName string) RateLimitGetKeyFunc {
	return func(r *http.Request) (key string, bypass bool, err error) {
		key = r.Header.Get(headerName)
		return key, key == "", nil
	}
}
