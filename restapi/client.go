/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"git.acronis.com/abc/go-libs/v2/log"
)

const (
	logKeyMethod = "method"
	logKeyURI    = "uri"
	logKeyStatus = "status"
)

// DoRequest allows to do HTTP requests and log some its details
func DoRequest(client *http.Client, req *http.Request, logger log.FieldLogger) (*http.Response, error) {
	logger.AtLevel(log.LevelDebug, func(logFn log.LogFunc) {
		logFn("sent request",
			log.String(logKeyMethod, req.Method),
			log.String(logKeyURI, req.URL.String()),
		)
	})

	resp, err := client.Do(req)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to do http request %s %s", req.Method, req.URL.String()),
			log.String(logKeyMethod, req.Method),
			log.String(logKeyURI, req.URL.String()),
			log.Error(err),
		)
		return nil, fmt.Errorf("do request: %w", err)
	}

	logger.AtLevel(log.LevelDebug, func(logFn log.LogFunc) {
		logFn("got response",
			log.String(logKeyMethod, req.Method),
			log.String(logKeyURI, req.URL.String()),
			log.Int(logKeyStatus, resp.StatusCode),
		)
	})
	return resp, nil
}

// DoRequestAndUnmarshalJSON allows doing HTTP requests, log some its details and unmarshal response
func DoRequestAndUnmarshalJSON(client *http.Client, req *http.Request, result interface{}, logger log.FieldLogger) error {
	resp, err := DoRequest(client, req, logger)
	// error is already logged in DoRequest
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("failed to close response body after doing http request",
				log.String(logKeyMethod, req.Method),
				log.String(logKeyURI, req.URL.String()),
				log.Error(err),
			)
		}
	}()
	logger.AtLevel(log.LevelDebug, func(logFn log.LogFunc) {
		logFn("got response",
			log.String(logKeyMethod, req.Method),
			log.String(logKeyURI, req.URL.String()),
			log.Int(logKeyStatus, resp.StatusCode),
		)
	})

	logger = logger.With(
		log.String(logKeyMethod, req.Method),
		log.String(logKeyURI, req.URL.String()),
		log.Int(logKeyStatus, resp.StatusCode),
	)

	var e = &ClientError{
		Method:     req.Method,
		URL:        req.URL,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 600 {
		buf, err := readResponseBody(resp, logger, e)
		if err != nil {
			return err
		}

		var apiErr ErrorResponseData
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			maxBodySize := 255
			if len(buf) < maxBodySize {
				maxBodySize = len(buf)
			}
			apiErr.Err = &Error{
				Code:    resp.Status,
				Message: fmt.Sprintf("%s received with unexpected Content-Type", http.StatusText(resp.StatusCode)),
				Debug: map[string]interface{}{
					"content-type": contentType,
					"body":         string(buf)[:maxBodySize],
				},
			}
		} else if err = json.Unmarshal(buf, &apiErr); err != nil {
			logger.Error("error unmarshaling error response", log.Error(err))
			return e.wrap("unmarshaling error response", err)
		}

		// default
		return e.wrap("error response", &apiErr)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if result != nil {
			buf, err := readResponseBody(resp, logger, e)
			if err != nil {
				return err
			}

			if err = json.Unmarshal(buf, &result); err != nil {
				logger.Error("error unmarshaling response", log.Error(err))
				return e.wrap("unmarshaling response", err)
			}
		}
		return nil
	}

	e.Message = "unexpected status code"
	return e
}

func readResponseBody(resp *http.Response, logger log.FieldLogger, e *ClientError) ([]byte, error) {
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("error reading response body", log.Error(err))
		return nil, e.wrap("reading response body", err)
	}
	if len(buf) == 0 {
		logger.Error("empty error response")
		e.Message = "empty response"
		return nil, e
	}
	return buf, nil
}

// NewJSONRequest performs JSON marshaling of the passed data and creates a new http.Request
func NewJSONRequest(method, url string, data interface{}) (*http.Request, error) {
	if data == nil {
		return nil, fmt.Errorf("data cannot be nil")
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return nil, fmt.Errorf("method %s is not allowed for json request", method)
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ContentTypeAppJSON)
	return req, nil
}
