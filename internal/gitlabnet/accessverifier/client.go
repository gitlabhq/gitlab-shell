package accessverifier

import (
	"context"
	"fmt"
	"net/http"

	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

const (
	sshProtocol = "ssh"
	anyChanges  = "_any"
)

type Client struct {
	client *client.GitlabNetClient
	config *config.Config
	host   string
}

type Request struct {
	Action        commandargs.CommandType `json:"action"`
	Repo          string                  `json:"project"`
	Changes       string                  `json:"changes"`
	Protocol      string                  `json:"protocol"`
	KeyId         string                  `json:"key_id,omitempty"`
	Username      string                  `json:"username,omitempty"`
	Krb5Principal string                  `json:"krb5principal,omitempty"`
	CheckIp       string                  `json:"check_ip,omitempty"`
	// NamespacePath is the full path of the namespace in which the authenticated
	// user is allowed to perform operation.
	NamespacePath string `json:"namespace_path,omitempty"`
}

type Gitaly struct {
	Repo     pb.Repository     `json:"repository"`
	Address  string            `json:"address"`
	Token    string            `json:"token"`
	Features map[string]string `json:"features"`
}

type CustomPayloadData struct {
	ApiEndpoints                            []string          `json:"api_endpoints"`
	Username                                string            `json:"gl_username"`
	PrimaryRepo                             string            `json:"primary_repo"`
	UserId                                  string            `json:"gl_id,omitempty"`
	RequestHeaders                          map[string]string `json:"request_headers"`
	GeoProxyDirectToPrimary                 bool              `json:"geo_proxy_direct_to_primary"`
	GeoProxyFetchDirectToPrimary            bool              `json:"geo_proxy_fetch_direct_to_primary"`
	GeoProxyFetchDirectToPrimaryWithOptions bool              `json:"geo_proxy_fetch_direct_to_primary_with_options"`
}

type CustomPayload struct {
	Action string            `json:"action"`
	Data   CustomPayloadData `json:"data"`
}

type Response struct {
	Success          bool          `json:"status"`
	Message          string        `json:"message"`
	Repo             string        `json:"gl_repository"`
	UserId           string        `json:"gl_id"`
	KeyType          string        `json:"gl_key_type"`
	KeyId            int           `json:"gl_key_id"`
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
	// GitRpcUrl is the url to fetch Git SSH data via HTTP
	GitRpcUrl        string `json:"git_rpc_url"`
	GitRpcAuthHeader string `json:"git_rpc_auth_header"`
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{client: client, config: config, host: client.HttpClient.Host}, nil
}

func (c *Client) Verify(ctx context.Context, args *commandargs.Shell, action commandargs.CommandType, repo string) (*Response, error) {
	request := &Request{
		Action:        action,
		Repo:          repo,
		Changes:       anyChanges,
		Protocol:      sshProtocol,
		NamespacePath: args.Env.NamespacePath,
	}

	if args.GitlabUsername != "" {
		request.Username = args.GitlabUsername
	} else if args.GitlabKrb5Principal != "" {
		request.Krb5Principal = args.GitlabKrb5Principal
	} else {
		request.KeyId = args.GitlabKeyId
	}

	request.CheckIp = gitlabnet.ParseIP(args.Env.RemoteAddr)

	host := c.getCellsAddress(ctx, repo)
	response, err := c.client.DoRequest(ctx, http.MethodPost, host, "/api/v4/internal/allowed", request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	return parse(response, args)
}

func parse(hr *http.Response, args *commandargs.Shell) (*Response, error) {
	response := &Response{}
	if err := gitlabnet.ParseJSON(hr, response); err != nil {
		return nil, err
	}

	if args.GitlabKeyId != "" {
		response.Who = "key-" + args.GitlabKeyId
	} else {
		response.Who = response.UserId
	}

	response.StatusCode = hr.StatusCode

	return response, nil
}

func (r *Response) IsCustomAction() bool {
	return r.StatusCode == http.StatusMultipleChoices
}

func (c *Client) getCellsAddress(ctx context.Context, repo string) string {
	cellsClient := c.config.Cells.Client
	if cellsClient == nil {
		return c.host
	}

	cell, err := cellsClient.Classify(ctx, repo)
	if err != nil {
		return c.host
	}

	return cell.Address
}
