package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

// ParseJSON decodes a successful (< 400) HTTP response body into dst, or
// returns a *client.APIError describing the failure. The error semantics
// deliberately match those of the old client package so that sub-clients
// migrated to this package do not need to change their error handling:
//
//   - 4xx/5xx with a JSON {"message":"…"} body → *client.APIError{Msg: message}
//   - 4xx/5xx with no decodable message       → *client.APIError{Msg: "Internal API error (N)"}
//   - 2xx with non-JSON body                  → errors.New("parsing failed")
func ParseJSON(resp *http.Response, dst any) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil || errResp.Message == "" {
			return &client.APIError{Msg: fmt.Sprintf("Internal API error (%d)", resp.StatusCode)}
		}
		return &client.APIError{Msg: errResp.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return errors.New("parsing failed")
	}
	return nil
}
