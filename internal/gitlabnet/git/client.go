// Package git provides functionality for interacting with Git repositories.
package git

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/labkit/v2/log"
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
	// PrepareRequest, if set, is called for every request after Headers and
	// method-specific headers (such as Content-Type and Accept) are applied, so it
	// can override them. Headers added later by the transport (User-Agent,
	// X-Forwarded-For, correlation and tracing) are appended or replaced there, so
	// they cannot be reliably set here. Implementations should only modify headers;
	// replacing Body or URL is not supported.
	PrepareRequest func(*http.Request) error
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

	if c.PrepareRequest != nil {
		if err := c.PrepareRequest(request); err != nil {
			// Unblock any writer feeding the body, as httpClient.Do would.
			if request.Body != nil {
				_ = request.Body.Close()
			}
			return nil, err
		}
	}

	// Over HTTP/2, closing a response body waits for the request body to finish
	// sending, but SSH stdin stays silent until git gets a reply. Canceling the
	// request before closing ends that wait.
	ctx, cancel := context.WithCancel(request.Context())
	request = request.WithContext(ctx)

	response, err := httpClient.Do(request) // #nosec G704 -- URL is constructed from configured GitLab internal API
	if err != nil {
		cancel()
		return nil, &client.APIError{Msg: repoUnavailableErrMsg}
	}

	response.Body = &cancelOnCloseBody{ReadCloser: response.Body, cancel: cancel}

	if response.StatusCode >= 400 {
		defer func() {
			if err := response.Body.Close(); err != nil {
				slog.ErrorContext(request.Context(), "Unable to close response body", log.ErrorMessage(err.Error()))
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

// cancelOnCloseBody cancels the request context before closing the response
// body, so Close doesn't wait on a request body that is still open.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	b.cancel()

	return b.ReadCloser.Close()
}
