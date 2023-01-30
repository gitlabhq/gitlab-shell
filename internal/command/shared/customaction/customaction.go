package customaction

import (
	"context"
	"errors"
	"io"
	"net/http"
	"mime/multipart"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
)

type Request struct {
	Data        accessverifier.CustomPayloadData `json:"data"`
	Output  io.Reader
}

type Response struct {
	Result  []byte `json:"result"`
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

		httpRequest, err := c.prepareRequest(ctx, client.AppendPath(endpoint), request)
		if err != nil {
			return err
		}

		if err := c.performRequest(ctx, client, httpRequest); err != nil {
			return err
		}

		// In the context of the git push sequence of events, it's necessary to read
		// stdin in order to capture output to pass onto subsequent commands
		if c.EOFSent {
			var w *io.PipeWriter
			request.Output, w = io.Pipe()

			go c.readFromStdin(w)
		} else {
			// output = c.readFromStdinNoEOF()
		}
		ctxlog.WithFields(log.Fields{
			"eof_sent":    c.EOFSent,
			// "stdin_bytes": len(output),
		}).Debug("customaction: processApiEndpoints: stdin buffered")
	}

	return nil
}

func (c *Command) prepareRequest(ctx context.Context, endpoint string, request *Request) (*http.Request, error) {
	body, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	go func() {
		writer.WriteField("data[gl_id]", request.Data.UserId)
		writer.WriteField("data[primary_repo]", request.Data.PrimaryRepo)

		if request.Output != nil {
			// Ignore errors, but may want to log them in a channel
			binaryPart, _ := writer.CreateFormFile("output", "git-receive-pack")
			io.Copy(binaryPart, request.Output)
		}

		writer.Close()
		pipeWriter.Close()
	}()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())

	return httpRequest, nil
}

func (c *Command) performRequest(ctx context.Context, client *client.GitlabNetClient, request *http.Request) error {
	response, err := client.DoRawRequest(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if _, err := io.Copy(c.ReadWriter.Out, response.Body); err != nil {
		return err
	}

	return nil
}

func (c *Command) readFromStdin(w *io.PipeWriter) {
	var needsPackData bool

	scanner := pktline.NewScanner(c.ReadWriter.In)
	for scanner.Scan() {
		line := scanner.Bytes()
		w.Write(line)

		if pktline.IsFlush(line) {
			break
		}

		if !needsPackData && !pktline.IsRefRemoval(line) {
			needsPackData = true
		}
	}

	if needsPackData {
		io.Copy(w, c.ReadWriter.In)
	}

	w.Close()
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
