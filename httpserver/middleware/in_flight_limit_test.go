/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"git.acronis.com/abc/go-libs/v2/testutil"
)

func TestInFlightLimitHandler_ServeHTTP(t *testing.T) {
	const errDomain = "MyService"

	makeReqAndRespRec := func() (*http.Request, *httptest.ResponseRecorder) {
		return httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()
	}

	type respInfo struct {
		code       int
		startTime  time.Time
		finishTime time.Time
	}

	t.Run("limit=1, backlogLimit=0, no key", func(t *testing.T) {
		acquired := make(chan struct{})
		reqContinued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-reqContinued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimit(1, errDomain)(next)

		respCode := make(chan int)
		go func() {
			// Do the first HTTP request.
			req, respRec := makeReqAndRespRec()
			handler.ServeHTTP(respRec, req)
			respCode <- respRec.Code
		}()
		<-acquired // Wait until the first HTTP request starts to be processed.
		block = false

		// Try to do the second HTTP request -> 503.
		req, respRec := makeReqAndRespRec()
		handler.ServeHTTP(respRec, req)
		testutil.RequireErrorInRecorder(t, respRec, http.StatusServiceUnavailable, errDomain, InFlightLimitErrCode)
		require.Empty(t, respRec.Header().Get("Retry-After"))

		close(reqContinued)                         // Let the first HTTP request be continued.
		require.Equal(t, http.StatusOK, <-respCode) // Wait until the second goroutine ends.

		// Now we can do the next HTTP request without any problem.
		req, respRec = makeReqAndRespRec()
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)
	})

	t.Run("limit=1, backlogLimit=1, no key", func(t *testing.T) {
		const backlogTimeout = time.Second
		acquired := make(chan struct{})
		req1Continued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-req1Continued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{BacklogLimit: 1, BacklogTimeout: backlogTimeout})(next)

		resp1Code := make(chan int)
		go func() {
			// Do the first HTTP request.
			req1, respRec1 := makeReqAndRespRec()
			handler.ServeHTTP(respRec1, req1)
			resp1Code <- respRec1.Code
		}()
		<-acquired // Wait until the first HTTP request starts to be processed.
		block = false

		// Try to do the second HTTP request -> 503.
		req2, respRec2 := makeReqAndRespRec()
		req2Start := time.Now()
		handler.ServeHTTP(respRec2, req2)
		testutil.RequireErrorInRecorder(t, respRec2, http.StatusServiceUnavailable, errDomain, InFlightLimitErrCode)
		require.Empty(t, respRec2.Header().Get("Retry-After"))
		require.WithinDurationf(t, req2Start.Add(backlogTimeout), time.Now(), time.Millisecond*50,
			"The second request should be backlogged for %s.", backlogTimeout.String())

		resp3Done := make(chan respInfo)
		go func() {
			// Do the third HTTP request.
			req3, respRec3 := makeReqAndRespRec()
			req3Start := time.Now()
			handler.ServeHTTP(respRec3, req3)
			resp3Done <- respInfo{respRec3.Code, req3Start, time.Now()}
		}()

		time.Sleep(backlogTimeout / 2)
		close(req1Continued)                         // Let the first HTTP request be continued.
		require.Equal(t, http.StatusOK, <-resp1Code) // Wait until the goroutine with 1th request ends.
		resp3Info := <-resp3Done                     // Wait until the goroutine with 3rd request ends.
		require.Equal(t, http.StatusOK, resp3Info.code)
		require.WithinDurationf(t, resp3Info.startTime.Add(backlogTimeout/2), resp3Info.finishTime, time.Millisecond*50,
			"The third request should be backlogged for %s.", (backlogTimeout / 2).String())

		// Now we can do the next HTTP request without any problem.
		req4, respRec4 := makeReqAndRespRec()
		handler.ServeHTTP(respRec4, req4)
		require.Equal(t, http.StatusOK, respRec4.Code)
	})

	t.Run("limit=100, backlogLimit=0, no key", func(t *testing.T) {
		const limit = 100
		const reqsNum = 500
		const reqDelay = 1 * time.Second
		var nextServedCount, respOKCount, respTooManyReqsCount, respUnexpectedCodeReqsCount atomic.Int32
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			nextServedCount.Inc()
			time.Sleep(reqDelay)
			rw.WriteHeader(http.StatusOK)
		})

		handler := InFlightLimit(limit, errDomain)(next)
		var wg sync.WaitGroup
		for i := 0; i < reqsNum; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, respRec := makeReqAndRespRec()
				handler.ServeHTTP(respRec, req)
				switch respRec.Code {
				case http.StatusOK:
					respOKCount.Inc()
				case http.StatusServiceUnavailable:
					respTooManyReqsCount.Inc()
				default:
					respUnexpectedCodeReqsCount.Inc()
				}
			}()
		}
		wg.Wait()

		// Check numbers
		require.Equal(t, limit, int(nextServedCount.Load()))
		require.Equal(t, limit, int(respOKCount.Load()))
		require.Equal(t, reqsNum-limit, int(respTooManyReqsCount.Load()))
		require.Equal(t, 0, int(respUnexpectedCodeReqsCount.Load()))

		// Sure we can do next http request.
		time.Sleep(reqDelay)
		req, respRec := makeReqAndRespRec()
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)
	})

	t.Run("limit=1, backlogLimit=0, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		acquired := make(chan struct{})
		reqContinued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-reqContinued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{
			GetKey:             makeInFlightLimitGetKeyByHeader(headerClientID),
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		respDone := make(chan int)
		go func() {
			// Do the first HTTP request.
			req, respRec := makeReqAndRespRec()
			req.Header.Set(headerClientID, "client-1")
			handler.ServeHTTP(respRec, req)
			respDone <- respRec.Code
		}()
		<-acquired // Wait until the first HTTP request starts to be processed.
		block = false

		// Try to do the second HTTP request with the same key -> 503.
		req, respRec := makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusTooManyRequests, respRec.Code)

		// Try to do the second HTTP request with the different key -> 200.
		req, respRec = makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-2")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)

		close(reqContinued)                         // Let the first HTTP request be continued.
		require.Equal(t, http.StatusOK, <-respDone) // Wait until the second goroutine ends.

		// Now we can do the next HTTP request without any problem.
		req, respRec = makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)
	})

	t.Run("limit=1, backlogLimit=1, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		const backlogTimeout = time.Second
		acquired := make(chan struct{})
		req1Continued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-req1Continued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{
			GetKey: func(r *http.Request) (string, bool, error) {
				key := r.Header.Get(headerClientID)
				return key, key == "", nil
			},
			ResponseStatusCode: http.StatusTooManyRequests,
			BacklogLimit:       1,
			BacklogTimeout:     backlogTimeout,
		})(next)

		resp1Done := make(chan int)
		go func() {
			// Do the 1st HTTP request.
			req1, respRec1 := makeReqAndRespRec()
			req1.Header.Set(headerClientID, "client-1")
			handler.ServeHTTP(respRec1, req1)
			resp1Done <- respRec1.Code
		}()
		<-acquired // Wait until the 1st HTTP request starts to be processed.
		block = false

		// Try to do the 2nd HTTP request with the same key -> 503.
		req2, respRec2 := makeReqAndRespRec()
		req2Start := time.Now()
		req2.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec2, req2)
		require.Equal(t, http.StatusTooManyRequests, respRec2.Code)
		require.WithinDurationf(t, req2Start.Add(backlogTimeout), time.Now(), time.Millisecond*50,
			"The second request should be backlogged for %s.", backlogTimeout.String())

		// Try to do the 3rd HTTP request with the different key -> 200.
		req3, respRec3 := makeReqAndRespRec()
		req3.Header.Set(headerClientID, "client-2")
		handler.ServeHTTP(respRec3, req3)
		require.Equal(t, http.StatusOK, respRec3.Code)

		resp4Done := make(chan respInfo)
		go func() {
			// Do the 4th HTTP request.
			req4, respRec4 := makeReqAndRespRec()
			req4.Header.Set(headerClientID, "client-1")
			req4Start := time.Now()
			handler.ServeHTTP(respRec4, req4)
			resp4Done <- respInfo{respRec4.Code, req4Start, time.Now()}
		}()

		time.Sleep(backlogTimeout / 2)
		close(req1Continued)                         // Let the 1st HTTP request be continued.
		require.Equal(t, http.StatusOK, <-resp1Done) // Wait until the 1st response finish.
		resp4Info := <-resp4Done                     // Wait until the 4th response finish.
		require.Equal(t, http.StatusOK, resp4Info.code)
		require.WithinDurationf(t, resp4Info.startTime.Add(backlogTimeout/2), resp4Info.finishTime, time.Millisecond*50,
			"The second request should be backlogged for %s.", (backlogTimeout / 2).String())

		// Now we can do the next HTTP request without any problem.
		req5, respRec5 := makeReqAndRespRec()
		req5.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec5, req5)
		require.Equal(t, http.StatusOK, respRec5.Code)
	})

	t.Run("limit=1, backlogLimit=0, by key, no bypass empty key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		acquired := make(chan struct{})
		req1Continued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-req1Continued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{
			GetKey: func(r *http.Request) (string, bool, error) {
				return r.Header.Get(headerClientID), false, nil
			},
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		resp1Done := make(chan int)
		go func() {
			// Do the 1st HTTP request.
			req1, respRec1 := makeReqAndRespRec()
			handler.ServeHTTP(respRec1, req1)
			resp1Done <- respRec1.Code
		}()
		<-acquired // Wait until the 1st HTTP request starts to be processed.
		block = false

		// Try to do the 2nd HTTP request with missing key.
		req2, respRec2 := makeReqAndRespRec()
		handler.ServeHTTP(respRec2, req2)
		require.Equal(t, http.StatusTooManyRequests, respRec2.Code)

		// Try to do the 3rd HTTP request with the different key -> 200.
		req3, respRec3 := makeReqAndRespRec()
		req3.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec3, req3)
		require.Equal(t, http.StatusOK, respRec3.Code)

		close(req1Continued)                         // Let the 1st HTTP request be continued.
		require.Equal(t, http.StatusOK, <-resp1Done) // Wait until the 1st response finish.

		// Now we can do the next HTTP request without any problem.
		req4, respRec4 := makeReqAndRespRec()
		handler.ServeHTTP(respRec4, req4)
		require.Equal(t, http.StatusOK, respRec4.Code)
	})

	t.Run("limit=100, backlogLimit=0, by key", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		const limitPerClient = 100
		const clientsNum = 10
		const reqsPerClient = 200
		const reqDelay = 2 * time.Second
		var nextServedCount atomic.Int32
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			nextServedCount.Inc()
			time.Sleep(reqDelay)
			rw.WriteHeader(http.StatusOK)
		})

		handler := InFlightLimitWithOpts(limitPerClient, errDomain, InFlightLimitOpts{
			GetKey:             makeInFlightLimitGetKeyByHeader(headerClientID),
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)
		respCodes := make([]struct {
			okCount                 atomic.Int32
			tooManyReqsCount        atomic.Int32
			unexpectedCodeReqsCount atomic.Int32
		}, clientsNum)
		var wg sync.WaitGroup
		for i := 0; i < clientsNum; i++ {
			for j := 0; j < reqsPerClient; j++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					req, respRec := makeReqAndRespRec()
					req.Header.Set(headerClientID, fmt.Sprintf("client-%d", clientIndex+1))
					handler.ServeHTTP(respRec, req)
					switch respRec.Code {
					case http.StatusOK:
						respCodes[clientIndex].okCount.Inc()
					case http.StatusTooManyRequests:
						respCodes[clientIndex].tooManyReqsCount.Inc()
					default:
						respCodes[clientIndex].unexpectedCodeReqsCount.Inc()
					}
				}(i)
			}
		}
		wg.Wait()

		// Check numbers.
		require.Equal(t, clientsNum*limitPerClient, int(nextServedCount.Load()))
		for i := 0; i < clientsNum; i++ {
			require.Equal(t, limitPerClient, int(respCodes[i].okCount.Load()))
			require.Equal(t, reqsPerClient-limitPerClient, int(respCodes[i].tooManyReqsCount.Load()))
			require.Equal(t, 0, int(respCodes[i].unexpectedCodeReqsCount.Load()))
		}

		// Sure we can do next http request.
		time.Sleep(reqDelay)
		req, respRec := makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)
	})

	t.Run("limit=1, backlogLimit=0, by key, keysZoneSize=1", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		acquired := make(chan struct{})
		reqContinued := make(chan struct{})
		block := true
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if block {
				close(acquired)
				<-reqContinued
			}
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{
			GetKey:             makeInFlightLimitGetKeyByHeader(headerClientID),
			MaxKeys:            1,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)

		respDone := make(chan int)
		go func() {
			// Do the first HTTP request.
			req, respRec := makeReqAndRespRec()
			req.Header.Set(headerClientID, "client-1")
			handler.ServeHTTP(respRec, req)
			respDone <- respRec.Code
		}()
		<-acquired // Wait until the first HTTP request starts to be processed.
		block = false

		// Try to do the second HTTP request with the different key -> 200.
		req, respRec := makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-2")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code)

		// Try to do the second HTTP request with the same key -> 200.
		req, respRec = makeReqAndRespRec()
		req.Header.Set(headerClientID, "client-1")
		handler.ServeHTTP(respRec, req)
		require.Equal(t, http.StatusOK, respRec.Code, "client-1 should was evicted by client-2 (the previous request)")

		close(reqContinued)                         // Let the first HTTP request be continued.
		require.Equal(t, http.StatusOK, <-respDone) // Wait until the second goroutine ends.
	})

	t.Run("limit=100, backlogLimit=0, by key, keysZoneSize=1 (many evictions in lru)", func(t *testing.T) {
		const headerClientID = "X-Client-ID"
		const limit = 1000
		const reqsNum = 2000
		var nextServedCount atomic.Int32
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			nextServedCount.Inc()
			rw.WriteHeader(http.StatusOK)
		})

		handler := InFlightLimitWithOpts(limit, errDomain, InFlightLimitOpts{
			GetKey:             makeInFlightLimitGetKeyByHeader(headerClientID),
			MaxKeys:            1,
			ResponseStatusCode: http.StatusTooManyRequests,
		})(next)
		var okCount, unexpectedCodeCount atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < reqsNum; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				req, respRec := makeReqAndRespRec()
				req.Header.Set(headerClientID, fmt.Sprintf("client-%d", i%2))
				handler.ServeHTTP(respRec, req)
				switch respRec.Code {
				case http.StatusOK:
					okCount.Inc()
				default:
					unexpectedCodeCount.Inc()
				}
			}(i)
		}
		wg.Wait()

		require.Equal(t, reqsNum, int(nextServedCount.Load()))
		require.Equal(t, reqsNum, int(okCount.Load()))
		require.Equal(t, 0, int(unexpectedCodeCount.Load()))
	})

	t.Run("Retry-After", func(t *testing.T) {
		const retryAfter = time.Second * 3

		acquired := make(chan struct{})
		reqContinued := make(chan struct{})
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			close(acquired)
			<-reqContinued
			rw.WriteHeader(http.StatusOK)
		})
		handler := InFlightLimitWithOpts(1, errDomain, InFlightLimitOpts{GetRetryAfter: func(r *http.Request) time.Duration {
			return retryAfter
		}})(next)

		respDone := make(chan int)
		go func() {
			req, respRec := makeReqAndRespRec()
			handler.ServeHTTP(respRec, req)
			respDone <- respRec.Code
		}()
		<-acquired // Wait until the first HTTP request starts to be processed.

		req, respRec := makeReqAndRespRec()
		handler.ServeHTTP(respRec, req)
		testutil.RequireErrorInRecorder(t, respRec, http.StatusServiceUnavailable, errDomain, InFlightLimitErrCode)
		require.Equal(t, strconv.Itoa(int(retryAfter.Seconds())), respRec.Header().Get("Retry-After"))

		close(reqContinued)                         // Let the first HTTP request be continued.
		require.Equal(t, http.StatusOK, <-respDone) // Wait until the second goroutine ends.
	})

	t.Run("dry-run mode", func(t *testing.T) {
		const limit = 1000
		const backlogLimit = 10000
		const reqsNum = 100000
		var nextServedCount, respOKCount, respTooManyReqsCount, respUnexpectedCodeReqsCount atomic.Int32
		next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			nextServedCount.Inc()
			rw.WriteHeader(http.StatusOK)
		})

		handler := InFlightLimitWithOpts(limit, errDomain, InFlightLimitOpts{
			DryRun:         true,
			BacklogLimit:   backlogLimit,
			BacklogTimeout: time.Millisecond * 10,
		})(next)
		var wg sync.WaitGroup
		for i := 0; i < reqsNum; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, respRec := makeReqAndRespRec()
				handler.ServeHTTP(respRec, req)
				switch respRec.Code {
				case http.StatusOK:
					respOKCount.Inc()
				case http.StatusServiceUnavailable:
					respTooManyReqsCount.Inc()
				default:
					respUnexpectedCodeReqsCount.Inc()
				}
			}()
		}
		wg.Wait()

		// Check numbers
		require.Equal(t, reqsNum, int(nextServedCount.Load()))
		require.Equal(t, reqsNum, int(respOKCount.Load()))
		require.Equal(t, 0, int(respTooManyReqsCount.Load()))
		require.Equal(t, 0, int(respUnexpectedCodeReqsCount.Load()))
	})
}

//nolint:unparam
func makeInFlightLimitGetKeyByHeader(headerName string) InFlightLimitGetKeyFunc {
	return func(r *http.Request) (key string, bypass bool, err error) {
		key = r.Header.Get(headerName)
		return key, key == "", nil
	}
}
