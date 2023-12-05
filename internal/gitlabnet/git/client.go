package git

import (
	"context"
	"io"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

var httpClient = &http.Client{
	Transport: client.NewTransport(client.DefaultTransport()),
}

const repoUnavailableErrMsg = "Remote repository is unavailable"

type Client struct {
	Url     string
	Headers map[string]string
}

func (c *Client) InfoRefs(ctx context.Context, service string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Url+"/info/refs?service="+service, nil)
	if err != nil {
		return nil, err
	}

	return c.do(request)
}

func (c *Client) ReceivePack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Url+"/git-receive-pack", body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/x-git-receive-pack-request")
	request.Header.Add("Accept", "application/x-git-receive-pack-result")

	return c.do(request)
}

func (c *Client) UploadPack(ctx context.Context, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Url+"/git-upload-pack", body)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/x-git-upload-pack-request")
	request.Header.Add("Accept", "application/x-git-upload-pack-result")

	return c.do(request)
}

func (c *Client) do(request *http.Request) (*http.Response, error) {
	for k, v := range c.Headers {
		request.Header.Add(k, v)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return nil, &client.ApiError{Msg: repoUnavailableErrMsg}
	}

	if response.StatusCode >= 400 {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, &client.ApiError{Msg: repoUnavailableErrMsg}
		}

		if len(body) > 0 {
			return nil, &client.ApiError{Msg: string(body)}
		}

		return nil, &client.ApiError{Msg: repoUnavailableErrMsg}
	}

	return response, nil
}
