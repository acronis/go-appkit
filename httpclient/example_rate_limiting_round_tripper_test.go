/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

/*
ExampleNewRateLimitingRoundTripper demonstrates the use of RateLimitingRoundTripper with default parameters.

Add "// Output:" in the end of the function and run:

	$ go test ./httpclient -v -run ExampleNewRateLimitingRoundTripper

Output will be like:

	[Req#1] 204 (0ms)
	[Req#2] 204 (502ms)
	[Req#3] 204 (497ms)
	[Req#4] 204 (500ms)
	[Req#5] 204 (503ms)
*/
func ExampleNewRateLimitingRoundTripper() {
	// Note: error handling is intentionally omitted so as not to overcomplicate the example.
	// It is strictly necessary to handle all errors in real code.

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	}))

	// Let's make transport that may do maximum 2 requests per second.
	tr, _ := NewRateLimitingRoundTripper(http.DefaultTransport, 2)
	httpClient := &http.Client{Transport: tr}

	start := time.Now()
	prev := time.Now()
	for i := 0; i < 5; i++ {
		resp, _ := httpClient.Get(server.URL)
		_ = resp.Body.Close()
		now := time.Now()
		_, _ = fmt.Fprintf(os.Stderr, "[Req#%d] %d (%dms)\n", i+1, resp.StatusCode, now.Sub(prev).Milliseconds())
		prev = now
	}
	delta := time.Since(start) - time.Second*2
	if delta > time.Millisecond*10 {
		fmt.Println("Total time is much greater than 2s")
	} else {
		fmt.Println("Total time is about 2s")
	}

	// Output: Total time is about 2s
}

// ExampleNewRateLimitingRoundTripperWithOpts demonstrates the use of RateLimitingRoundTripper.
func ExampleNewRateLimitingRoundTripperWithOpts() {
	// Note: error handling is intentionally omitted so as not to overcomplicate the example.
	// It is strictly necessary to handle all errors in real code.

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	}))

	// Let's make transport that may do maximum 2 requests per second.
	tr, _ := NewRateLimitingRoundTripperWithOpts(http.DefaultTransport, 2, RateLimitingRoundTripperOpts{
		WaitTimeout: time.Millisecond * 100, // Wait maximum 100ms.
	})
	httpClient := &http.Client{Transport: tr}

	for i := 0; i < 2; i++ {
		resp, err := httpClient.Get(server.URL)
		if err != nil {
			var waitErr *RateLimitingWaitError
			if errors.As(err, &waitErr) {
				fmt.Printf("trying to do too many requests")
			}
			continue
		}
		_ = resp.Body.Close()
	}

	// Output: trying to do too many requests
}
