package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckClient_Check_Success(t *testing.T) {
	expectedResponse := HealthcheckResponse{
		APIVersion:     "1.0",
		GitlabVersion:  "15.0.0",
		GitlabRevision: "abc123",
		Redis:          true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	defer server.Close()

	cfg := &Config{
		GitlabURL: server.URL,
		Secret:    "test-secret",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	hc := NewHealthcheckClient(client)
	resp, err := hc.Check(context.Background())

	if err != nil {
		t.Fatalf("Check() returned unexpected error: %v", err)
	}

	if resp.Redis != expectedResponse.Redis {
		t.Errorf("Redis field mismatch: got %v, want %v", resp.Redis, expectedResponse.Redis)
	}

	if resp.APIVersion != expectedResponse.APIVersion {
		t.Errorf("APIVersion mismatch: got %s, want %s", resp.APIVersion, expectedResponse.APIVersion)
	}
}

func TestHealthcheckClient_Check_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer server.Close()

	cfg := &Config{
		GitlabURL: server.URL,
		Secret:    "test-secret",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	hc := NewHealthcheckClient(client)
	_, err = hc.Check(context.Background())

	if err == nil {
		t.Fatal("Check() expected error for non-OK status, got nil")
	}
}

func TestHealthcheckClient_Check_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	cfg := &Config{
		GitlabURL: server.URL,
		Secret:    "test-secret",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	hc := NewHealthcheckClient(client)
	_, err = hc.Check(context.Background())

	if err == nil {
		t.Fatal("Check() expected error for invalid JSON, got nil")
	}
}
