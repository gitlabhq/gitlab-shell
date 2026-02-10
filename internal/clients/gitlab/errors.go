package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIError represents an API error
type APIError struct {
	Msg string
}

func (e *APIError) Error() string {
	return e.Msg
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Message string `json:"message"`
}

func parseError(resp *http.Response, respErr error) error {
	if resp == nil || respErr != nil {
		return &APIError{"Internal API unreachable"}
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	parsedResponse := &ErrorResponse{}

	if err := json.NewDecoder(resp.Body).Decode(parsedResponse); err != nil {
		return &APIError{fmt.Sprintf("Internal API error (%v)", resp.StatusCode)}
	}
	return &APIError{parsedResponse.Message}
}
