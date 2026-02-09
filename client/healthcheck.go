package client

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	checkPath = "/check"
)

// HealthResponse contains healthcheck endpoint response data
type HealthResponse struct {
	APIVersion     string `json:"api_version"`
	GitlabVersion  string `json:"gitlab_version"`
	GitlabRevision string `json:"gitlab_rev"`
	Redis          bool   `json:"redis"`
}

func (g *GitlabNetClient) CheckHealth(ctx context.Context) (*HealthResponse, error) {
	resp, err := g.Get(ctx, checkPath)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = resp.Body.Close()
	}()

	response := &HealthResponse{}
	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return nil, fmt.Errorf("parsing failed")
	}

	return response, nil
}
