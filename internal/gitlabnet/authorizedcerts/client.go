// Package authorizedcerts implements functions for authorizing users with ssh certificates
package authorizedcerts

import (
	"context"
	"fmt"
	"net/url"

	"github.com/open-feature/go-sdk/openfeature"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

const (
	authorizedCertsPath = "/authorized_certs"

	// useNewClientFlag reuses the healthcheck feature-flag key. The healthcheck
	// call site was un-gated, freeing this key to validate the unified gitlab
	// client against real production traffic via the authorized_certs path. The
	// "healthcheck" in the name is an intentional, documented reuse: this key now
	// gates the authorized_certs client selection, not the health check.
	useNewClientFlag = "use_new_healthcheck_client"
)

// authorizedCertsEvalCtx is the feature-flag evaluation context. A constant
// targeting key is used because client selection is not per-user.
var authorizedCertsEvalCtx = openfeature.NewEvaluationContext("authorized_certs", nil)

// Client wraps both the legacy and the new gitlab client and selects between
// them via feature flag.
type Client struct {
	config    *config.Config
	client    *client.GitlabNetClient
	newClient *gitlab.Client
	resolver  *topology.Resolver
}

// Response contains the json response from authorized_certs
type Response struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
}

// NewClient instantiates a Client with config
func NewClient(config *config.Config) (*Client, error) {
	c, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	newClient, err := gitlab.New(&gitlab.Config{
		GitlabURL:          config.GitlabURL,
		User:               config.HTTPSettings.User,
		Password:           config.HTTPSettings.Password,
		Secret:             config.Secret,
		CaFile:             config.HTTPSettings.CaFile,
		CaPath:             config.HTTPSettings.CaPath,
		ReadTimeoutSeconds: config.HTTPSettings.ReadTimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating gitlab client: %v", err)
	}

	return &Client{
		config:    config,
		client:    c,
		newClient: newClient,
		resolver:  config.NewTopologyResolver(),
	}, nil
}

// GetByKey makes a request to authorized_certs for the namespace configured with a cert that matches fingerprint
func (c *Client) GetByKey(ctx context.Context, userID, fingerprint string) (*Response, error) {
	// The fingerprint here is the signing CA's SHA256 hash (without the
	// "SHA256:" prefix). Resolve the owning cell once so both the old and new
	// client paths route identically; only the HTTP client implementation
	// differs by flag state.
	host := c.resolver.HostForSSHFingerprint(ctx, fingerprint)

	if c.useNewClient(ctx) {
		gc := c.newClient
		if host != "" {
			gc = gc.WithHost(host)
		}
		resp, err := gitlab.NewAuthorizedCertsClient(gc).GetByKey(ctx, userID, fingerprint)
		if err != nil {
			return nil, err
		}
		return &Response{Username: resp.Username, Namespace: resp.Namespace}, nil
	}

	return c.getByKeyLegacy(ctx, userID, fingerprint, host)
}

// getByKeyLegacy issues the request via the legacy client.GitlabNetClient.
func (c *Client) getByKeyLegacy(ctx context.Context, userID, fingerprint, host string) (*Response, error) {
	httpClient := c.client
	if host != "" {
		httpClient = c.client.WithHost(host)
	}

	path, err := pathWithKey(userID, fingerprint)
	if err != nil {
		return nil, err
	}

	response, err := httpClient.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	parsedResponse := &Response{}
	if err := gitlabnet.ParseJSON(response, parsedResponse); err != nil {
		return nil, err
	}

	return parsedResponse, nil
}

// useNewClient reports whether the unified gitlab client should be used. It
// defaults to false (legacy client) when no evaluator is present or the flag
// evaluation errors.
func (c *Client) useNewClient(ctx context.Context) bool {
	evaluator := command.FeatureFlagEvaluatorFromContext(ctx)
	if evaluator == nil {
		return false
	}

	details, err := evaluator.BooleanValueDetails(ctx, useNewClientFlag, false, authorizedCertsEvalCtx)
	if err != nil {
		log.FromContext(ctx).WarnContext(ctx, "authorized_certs FF evaluation failed; using legacy client", log.Error(err))
		return false
	}
	return details.Value
}

func pathWithKey(userID, fingerprint string) (string, error) {
	u, err := url.Parse(authorizedCertsPath)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("key", fingerprint)
	params.Set("user_identifier", userID)
	u.RawQuery = params.Encode()

	return u.String(), nil
}
