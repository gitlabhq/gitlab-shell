package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HealthcheckResponse contains healthcheck endpoint response data.
type HealthcheckResponse struct {
	APIVersion     string `json:"api_version"`
	GitlabVersion  string `json:"gitlab_version"`
	GitlabRevision string `json:"gitlab_rev"`
	Redis          bool   `json:"redis"`
}

// HealthcheckClient wraps the gitlab Client for healthcheck requests.
type HealthcheckClient struct {
	client *Client
}

// NewHealthcheckClient creates a new healthcheck client from the gitlab Client.
func NewHealthcheckClient(client *Client) *HealthcheckClient {
	return &HealthcheckClient{client: client}
}

// Check makes a GET request to the healthcheck endpoint and returns the response.
func (hc *HealthcheckClient) Check(ctx context.Context) (*HealthcheckResponse, error) {
	resp, err := hc.client.Get(ctx, "/check")
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("healthcheck endpoint returned status %d", resp.StatusCode)
	}

	var response HealthcheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode healthcheck response: %w", err)
	}

	return &response, nil
}
