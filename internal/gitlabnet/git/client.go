// Package git provides functionality for interacting with Git repositories.
package git

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/labkit/fields"
)

var httpClient = &http.Client{
	Transport: client.NewTransport(client.DefaultTransport()),
}

const (
	repoUnavailableErrMsg = "Remote repository is unavailable"
	sshUploadPackPath     = "/ssh-upload-pack"
	sshReceivePackPath    = "/ssh-receive-pack"
)

// Client represents a client for interacting with Git repositories.
type Client struct {
	URL     string
	Headers map[string]string
}

// InfoRefs retrieves information about the Git repository references.
func (c *Client) InfoRefs(ctx context.Context, service string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/info/refs?service="+service, nil)
	if err != nil {
		return nil, err
	}

	return c.do(request)
}

// ReceivePack sends a Git push request to the server.
func (c *Client) ReceivePack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/git-receive-pack", body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/x-git-receive-pack-request")
	request.Header.Add("Accept", "application/x-git-receive-pack-result")

	return c.do(request)
}

// UploadPack sends a Git fetch request to the server.
func (c *Client) UploadPack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/git-upload-pack", body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/x-git-upload-pack-request")
	request.Header.Add("Accept", "application/x-git-upload-pack-result")

	return c.do(request)
}

// SSHUploadPack sends a SSH Git fetch request to the server.
func (c *Client) SSHUploadPack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+sshUploadPackPath, body)
	if err != nil {
		return nil, err
	}

	return c.do(request)
}

// SSHReceivePack sends a SSH Git push request to the server.
func (c *Client) SSHReceivePack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+sshReceivePackPath, body)
	if err != nil {
		return nil, err
	}

	return c.do(request)
}

func (c *Client) do(request *http.Request) (*http.Response, error) {
	for k, v := range c.Headers {
		request.Header.Add(k, v)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, &client.APIError{Msg: repoUnavailableErrMsg}
	}

	if response.StatusCode >= 400 {
		defer func() {
			if err := response.Body.Close(); err != nil {
				slog.ErrorContext(request.Context(), "Unable to close response body", slog.String(fields.ErrorMessage, err.Error()))
			}
		}()

		body, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, &client.APIError{Msg: repoUnavailableErrMsg}
		}

		if len(body) > 0 {
			return nil, &client.APIError{Msg: string(body)}
		}

		return nil, &client.APIError{Msg: repoUnavailableErrMsg}
	}

	return response, nil
}
