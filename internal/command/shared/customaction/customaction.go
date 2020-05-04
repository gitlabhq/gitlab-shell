package customaction

import (
	"bytes"
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/client"

	"io"
	"net/http"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/pktline"
)

type Request struct {
	SecretToken []byte                           `json:"secret_token"`
	Data        accessverifier.CustomPayloadData `json:"data"`
	Output      []byte                           `json:"output"`
}

type Response struct {
	Result  []byte `json:"result"`
	Message string `json:"message"`
}

type Command struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	EOFSent    bool
}

func (c *Command) Execute(response *accessverifier.Response) error {
	data := response.Payload.Data
	apiEndpoints := data.ApiEndpoints

	if len(apiEndpoints) == 0 {
		return errors.New("Custom action error: Empty API endpoints")
	}

	return c.processApiEndpoints(response)
}

func (c *Command) processApiEndpoints(response *accessverifier.Response) error {
	client, err := gitlabnet.GetClient(c.Config)

	if err != nil {
		return err
	}

	data := response.Payload.Data
	request := &Request{Data: data}
	request.Data.UserId = response.Who

	for _, endpoint := range data.ApiEndpoints {
		fields := log.Fields{
			"primary_repo": data.PrimaryRepo,
			"endpoint":     endpoint,
		}

		log.WithFields(fields).Info("Performing custom action")

		response, err := c.performRequest(client, endpoint, request)
		if err != nil {
			return err
		}

		// Print to os.Stdout the result contained in the response
		//
		if err = c.displayResult(response.Result); err != nil {
			return err
		}

		// In the context of the git push sequence of events, it's necessary to read
		// stdin in order to capture output to pass onto subsequent commands
		//
		var output []byte

		if c.EOFSent {
			output, err = c.readFromStdin()
			if err != nil {
				return err
			}
		} else {
			output = c.readFromStdinNoEOF()
		}

		request.Output = output
	}

	return nil
}

func (c *Command) performRequest(client *client.GitlabNetClient, endpoint string, request *Request) (*Response, error) {
	response, err := client.DoRequest(http.MethodPost, endpoint, request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	cr := &Response{}
	if err := gitlabnet.ParseJSON(response, cr); err != nil {
		return nil, err
	}

	return cr, nil
}

func (c *Command) readFromStdin() ([]byte, error) {
	output := new(bytes.Buffer)
	_, err := io.Copy(output, c.ReadWriter.In)

	return output.Bytes(), err
}

func (c *Command) readFromStdinNoEOF() []byte {
	var output []byte

	scanner := pktline.NewScanner(c.ReadWriter.In)
	for scanner.Scan() {
		line := scanner.Bytes()
		output = append(output, line...)

		if pktline.IsDone(line) {
			break
		}
	}

	return output
}

func (c *Command) displayResult(result []byte) error {
	_, err := io.Copy(c.ReadWriter.Out, bytes.NewReader(result))
	return err
}
