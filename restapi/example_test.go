/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	golog "log"
	"net/http"
	"net/http/httptest"
	"path"

	"git.acronis.com/abc/go-libs/v2/log"
)

func ExampleRespondJSON() {
	http.HandleFunc("/endpoint", func(w http.ResponseWriter, r *http.Request) {
		type User struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		RespondJSON(w, &User{Name: "Bob", Age: 12}, log.NewDisabledLogger())
	})
}

func ExampleRespondError() {
	http.HandleFunc("/endpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			apiErr := NewError("MyService", "methodNotAllowed", "Only GET HTTP method is allowed.")
			RespondError(w, http.StatusMethodNotAllowed, apiErr, log.NewDisabledLogger())
			return
		}
	})
}

func ExampleDecodeRequestJSON() {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	const domain = "MyService"

	http.HandleFunc("/endpoint", func(w http.ResponseWriter, r *http.Request) {
		var user User
		if err := DecodeRequestJSON(r, &user); err != nil {
			logger := log.NewDisabledLogger()

			var reqErr *MalformedRequestError
			if errors.As(err, &reqErr) {
				RespondMalformedRequestError(w, domain, reqErr, logger)
				return
			}

			RespondError(w, http.StatusInternalServerError, NewInternalError(domain), logger)
			return
		}
	})
}

// ExampleServiceClient is example for DoRequestAndUnmarshalJSON
type ExampleServiceClient struct {
	baseURL    string
	httpClient *http.Client
	logger     log.FieldLogger
}

func NewExampleClient(baseURL string, httpClient *http.Client, logger log.FieldLogger) *ExampleServiceClient {
	return &ExampleServiceClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		logger:     logger,
	}
}

func (c *ExampleServiceClient) url(requestPath string) string {
	return path.Join(c.baseURL, requestPath)
}

type ExampleClientResponse struct {
	ID string
}

func (c *ExampleServiceClient) Create(event interface{}) (*ExampleClientResponse, error) {
	req, err := NewJSONRequest(http.MethodPost, c.url("/events"), event)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var res *ExampleClientResponse
	err = DoRequestAndUnmarshalJSON(c.httpClient, req, &res, c.logger)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return res, err
}

func (c *ExampleServiceClient) Delete(id string) error {
	req, err := NewJSONRequest(http.MethodDelete, c.url(fmt.Sprintf("/events/%s", id)), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	err = DoRequestAndUnmarshalJSON(c.httpClient, req, nil, c.logger)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	return err
}

func ExampleClient() {
	logger := log.NewDisabledLogger()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			resp := ExampleClientResponse{"123"}
			buf, err := json.Marshal(resp)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, err = rw.Write(buf)
			if err != nil {
				logger.Error(err.Error())
				return
			}
			rw.WriteHeader(http.StatusOK)

		case r.Method == http.MethodDelete:
			rw.WriteHeader(http.StatusNoContent)
		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := NewExampleClient(path.Join(server.URL, "/api/v1"), &http.Client{}, logger)

	event := struct {
		Name string
	}{
		Name: "Alarm",
	}
	resp, err := client.Create(&event)
	if err != nil {
		golog.Fatal(err)
		return
	}
	err = client.Delete(resp.ID)
	if err != nil {
		golog.Fatal(err)
		return
	}
}
