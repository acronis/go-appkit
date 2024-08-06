/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"time"
)

/*
ExampleNewRetryableRoundTripper demonstrates the use of RetryableRoundTripper with default parameters.

To execute this example:
1) Add "// Output: Got 200 after 10 retry attempts" in the end of function.
2) Run:

	$ go test ./httpclient -v -run ExampleNewRetryableRoundTripper

Stderr will contain something like this:

	[Req#1] wait time: 0.001s
	[Req#2] wait time: 1.109s
	[Req#3] wait time: 2.882s
	[Req#4] wait time: 4.661s
	[Req#5] wait time: 7.507s
	[Req#6] wait time: 14.797s
	[Req#7] wait time: 37.984s
	[Req#8] wait time: 33.942s
	[Req#9] wait time: 39.394s
	[Req#10] wait time: 35.820s
	[Req#11] wait time: 48.060s
*/
func ExampleNewRetryableRoundTripper() {
	// Note: error handling is intentionally omitted so as not to overcomplicate the example.
	// It is strictly necessary to handle all errors in real code.

	reqTimes := make(chan time.Time, DefaultMaxRetryAttempts+1)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		reqTimes <- time.Now()
		if n, err := strconv.Atoi(r.Header.Get(RetryAttemptNumberHeader)); err == nil && n == DefaultMaxRetryAttempts {
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte("ok, you win..."))
			return
		}
		rw.WriteHeader(http.StatusServiceUnavailable)
	}))

	prevTime := time.Now()

	// Create RetryableRoundTripper with default params:
	//	+ maximum retry attempts = 10 (DefaultMaxRetryAttempts)
	//	+ respect Retry-After response HTTP header
	//	+ exponential backoff policy (DefaultBackoffPolicy, multiplier = 2, initial interval = 1s)
	tr, _ := NewRetryableRoundTripper(http.DefaultTransport)
	httpClient := &http.Client{Transport: tr}

	resp, err := httpClient.Get(server.URL)
	if err != nil {
		log.Fatal(err)
	}
	_ = resp.Body.Close()
	close(reqTimes)

	reqsCount := 0
	for reqTime := range reqTimes {
		reqsCount++
		_, _ = fmt.Fprintf(os.Stderr, "[Req#%d] wait time: %.3fs\n", reqsCount, reqTime.Sub(prevTime).Seconds())
		prevTime = reqTime
	}
	doneRetryAttempts := reqsCount - 1
	fmt.Printf("Got %d after %d retry attempts", resp.StatusCode, doneRetryAttempts)
}
