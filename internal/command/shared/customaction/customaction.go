package customaction

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
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

func (c *Command) Execute(ctx context.Context, response *accessverifier.Response) error {
	data := response.Payload.Data
	apiEndpoints := data.ApiEndpoints

	if len(apiEndpoints) == 0 {
		return errors.New("Custom action error: Empty API endpoints")
	}

	return c.processApiEndpoints(ctx, response)
}

func (c *Command) processApiEndpoints(ctx context.Context, response *accessverifier.Response) error {
	client, err := gitlabnet.GetClient(c.Config)

	if err != nil {
		return err
	}

	data := response.Payload.Data
	request := &Request{Data: data}
	request.Data.UserId = response.Who

	for _, endpoint := range data.ApiEndpoints {
		ctxlog := log.WithContextFields(ctx, log.Fields{
			"primary_repo": data.PrimaryRepo,
			"endpoint":     endpoint,
		})

		ctxlog.Info("customaction: processApiEndpoints: Performing custom action")

		response, err := c.performRequest(ctx, client, endpoint, request)
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
		ctxlog.WithFields(log.Fields{
			"eof_sent":    c.EOFSent,
			"stdin_bytes": len(output),
		}).Debug("customaction: processApiEndpoints: stdin buffered")

		request.Output = output
	}

	return nil
}

func (c *Command) performRequest(ctx context.Context, client *client.GitlabNetClient, endpoint string, request *Request) (*Response, error) {
	response, err := client.DoRequest(ctx, http.MethodPost, endpoint, request)
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
	var output []byte
	var needsPackData bool

	scanner := pktline.NewScanner(c.ReadWriter.In)
	for scanner.Scan() {
		line := scanner.Bytes()
		output = append(output, line...)

		if pktline.IsFlush(line) {
			break
		}

		if !needsPackData && !pktline.IsRefRemoval(line) {
			needsPackData = true
		}
	}

	if needsPackData {
		packData := new(bytes.Buffer)
		_, err := io.Copy(packData, c.ReadWriter.In)

		output = append(output, packData.Bytes()...)
		return output, err
	} else {
		return output, nil
	}
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
