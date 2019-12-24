package accessverifier

import (
	"fmt"
	"net/http"

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
)

const (
	protocol   = "ssh"
	anyChanges = "_any"
)

type Client struct {
	client *gitlabnet.GitlabClient
}

type Request struct {
	Action   commandargs.CommandType `json:"action"`
	Repo     string                  `json:"project"`
	Changes  string                  `json:"changes"`
	Protocol string                  `json:"protocol"`
	KeyId    string                  `json:"key_id,omitempty"`
	Username string                  `json:"username,omitempty"`
	CheckIp  string                  `json:"check_ip,omitempty"`
}

type Gitaly struct {
	Repo     pb.Repository     `json:"repository"`
	Address  string            `json:"address"`
	Token    string            `json:"token"`
	Features map[string]string `json:"features"`
}

type CustomPayloadData struct {
	ApiEndpoints []string `json:"api_endpoints"`
	Username     string   `json:"gl_username"`
	PrimaryRepo  string   `json:"primary_repo"`
	UserId       string   `json:"gl_id,omitempty"`
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
	Username         string        `json:"gl_username"`
	GitConfigOptions []string      `json:"git_config_options"`
	Gitaly           Gitaly        `json:"gitaly"`
	GitProtocol      string        `json:"git_protocol"`
	Payload          CustomPayload `json:"payload"`
	ConsoleMessages  []string      `json:"gl_console_messages"`
	Who              string
	StatusCode       int
}

func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating http client: %v", err)
	}

	return &Client{client: client}, nil
}

func (c *Client) Verify(args *commandargs.Shell, action commandargs.CommandType, repo string) (*Response, error) {
	request := &Request{Action: action, Repo: repo, Protocol: protocol, Changes: anyChanges}

	if args.GitlabUsername != "" {
		request.Username = args.GitlabUsername
	} else {
		request.KeyId = args.GitlabKeyId
	}

	request.CheckIp = sshenv.LocalAddr()

	response, err := c.client.Post("/allowed", request)
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
