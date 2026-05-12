package gitlab

import "context"

const healthcheckPath = "/check"

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
	resp, err := hc.client.Get(ctx, healthcheckPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var response HealthcheckResponse
	if err := ParseJSON(resp, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
