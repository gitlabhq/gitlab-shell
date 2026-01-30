// Package accessverifier provides functionality for verifying access to GitLab resources
package accessverifier

import (
	"context"
	"fmt"
	"net/http"

	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	sshProtocol = "ssh"
	anyChanges  = "_any"
)

// Client is a client for accessing resources
type Client struct {
	client *client.GitlabNetClient
}

// Request represents a request for accessing resources
type Request struct {
	Action        commandargs.CommandType `json:"action"`
	Repo          string                  `json:"project"`
	Changes       string                  `json:"changes"`
	Protocol      string                  `json:"protocol"`
	KeyID         string                  `json:"key_id,omitempty"`
	Username      string                  `json:"username,omitempty"`
	Krb5Principal string                  `json:"krb5principal,omitempty"`
	CheckIP       string                  `json:"check_ip,omitempty"`
	// NamespacePath is the full path of the namespace in which the authenticated
	// user is allowed to perform operation.
	NamespacePath string `json:"namespace_path,omitempty"`
}

// Gitaly represents Gitaly server information
type Gitaly struct {
	Repo     pb.Repository     `json:"repository"`
	Address  string            `json:"address"`
	Token    string            `json:"token"`
	Features map[string]string `json:"features"`
}

// CustomPayloadData represents custom payload data
type CustomPayloadData struct {
	APIEndpoints                    []string          `json:"api_endpoints"`
	Username                        string            `json:"gl_username"`
	PrimaryRepo                     string            `json:"primary_repo"`
	UserID                          string            `json:"gl_id,omitempty"`
	RequestHeaders                  map[string]string `json:"request_headers"`
	GeoProxyFetchSSHDirectToPrimary bool              `json:"geo_proxy_fetch_ssh_direct_to_primary"`
	GeoProxyPushSSHDirectToPrimary  bool              `json:"geo_proxy_push_ssh_direct_to_primary"`
}

// CustomPayload represents a custom payload
type CustomPayload struct {
	Action string            `json:"action"`
	Data   CustomPayloadData `json:"data"`
}

// Response represents a response from GitLab
type Response struct {
	Success          bool          `json:"status"`
	Message          string        `json:"message"`
	Repo             string        `json:"gl_repository"`
	UserID           string        `json:"gl_id"`
	KeyType          string        `json:"gl_key_type"`
	KeyID            int           `json:"gl_key_id"`
	ProjectID        int           `json:"gl_project_id"`
	RootNamespaceID  int           `json:"gl_root_namespace_id"`
	Username         string        `json:"gl_username"`
	GitConfigOptions []string      `json:"git_config_options"`
	Gitaly           Gitaly        `json:"gitaly"`
	GitProtocol      string        `json:"git_protocol"`
	Payload          CustomPayload `json:"payload"`
	ConsoleMessages  []string      `json:"gl_console_messages"`
	Who              string
	StatusCode       int
	// NeedAudit indicates whether git event should be audited to rails.
	NeedAudit bool `json:"need_audit"`
}

// NewClient creates a new instance of Client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{client: client}, nil
}

// Verify verifies access to a GitLab resource
func (c *Client) Verify(ctx context.Context, args *commandargs.Shell, action commandargs.CommandType, repo string) (*Response, error) {
	request := &Request{
		Action:        action,
		Repo:          repo,
		Changes:       anyChanges,
		Protocol:      sshProtocol,
		NamespacePath: args.Env.NamespacePath,
	}

	switch {
	case args.GitlabUsername != "":
		request.Username = args.GitlabUsername
	case args.GitlabKrb5Principal != "":
		request.Krb5Principal = args.GitlabKrb5Principal
	default:
		request.KeyID = args.GitlabKeyID
	}

	request.CheckIP = gitlabnet.ParseIP(args.Env.RemoteAddr)

	response, err := c.client.Post(ctx, "/allowed", request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	return parse(response, args)
}

func parse(hr *http.Response, args *commandargs.Shell) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	if args.GitlabKeyID != "" {
		response.Who = "key-" + args.GitlabKeyID
	} else {
		response.Who = response.UserID
	}

	response.StatusCode = hr.StatusCode

	return response, nil
}

// IsCustomAction checks if the response indicates a custom action
func (r *Response) IsCustomAction() bool {
	return r.StatusCode == http.StatusMultipleChoices
}
