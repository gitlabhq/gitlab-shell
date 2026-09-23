package githttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"

	clientpkg "gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

const testSecret = "test-secret-for-cells"

const (
	testGitProtocolVersion = "version=2"
	testGitalyToken        = "token"
)

func TestCellsCommandsExecute(t *testing.T) {
	responseBody := "cell-response"

	testCases := []struct {
		desc         string
		input        string
		expectedBody string
		expectedPath string
	}{
		{
			desc:         "protocol v2 fetch gets exactly one trailing flush",
			input:        fetchV2Request,
			expectedBody: fetchV2Request,
			expectedPath: "/group/project.git/ssh-upload-pack",
		},
		{
			desc:         "protocol v2 ls-refs without done forwards through EOF",
			input:        lsRefsV2Request,
			expectedBody: lsRefsV2Request,
			expectedPath: "/group/project.git/ssh-upload-pack",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cellServer, captured := startCapturingCellServer(t, responseBody)
			cfg := cellsTestConfig(t)

			output := &bytes.Buffer{}
			input := strings.NewReader(tc.input)

			err := NewCellsPullCommand(
				cfg,
				&readwriter.ReadWriter{Out: output, In: input},
				&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion}},
				cellsTestResponse(cellServer.URL),
			).Execute(context.Background())

			require.NoError(t, err)
			assert.Equal(t, responseBody, output.String())
			assert.Equal(t, tc.expectedPath, captured.path)
			assert.NotEmpty(t, captured.headers.Get("Gitlab-Shell-Api-Request"))
			assert.Equal(t, testGitProtocolVersion, captured.headers.Get("Git-Protocol"))
			assert.Equal(t, tc.expectedBody, string(captured.body))
		})
	}
}

func TestCellsPullClosesRequestBodyAfterDone(t *testing.T) {
	cellServer, captured := startCapturingCellServer(t, "cell-response")
	inputReader, inputWriter := io.Pipe()
	t.Cleanup(func() { inputWriter.Close() })

	command := NewCellsPullCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: io.Discard, In: inputReader},
		&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion}},
		cellsTestResponse(cellServer.URL),
	)
	result := make(chan error, 1)
	go func() {
		result <- command.Execute(context.Background())
	}()

	request := pktLine("want e56497bb5f03a90a51293fc6d516788730953899\n") + string(pktline.PktDone())
	_, err := io.WriteString(inputWriter, request)
	require.NoError(t, err)

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.Fail(t, "Cells pull waited for SSH EOF after done")
	}

	assert.Equal(t, request+string(pktline.PktFlush()), string(captured.body))
}

func TestCellsPushGatesStreamingOnClientInput(t *testing.T) {
	advertisement := pktLine("e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") + string(pktline.PktFlush())
	reportStatus := pktLine("unpack ok\n") + string(pktline.PktFlush())
	testCases := []struct {
		desc                   string
		input                  string
		responseBodies         []string
		wantBodies             []string
		wantOutput             string
		gateAfterFirstByteRead bool
	}{
		{
			desc:                   "commands and pack",
			input:                  pktLine("0000000000000000000000000000000000000000 e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") + string(pktline.PktFlush()) + "PACK receive-pack bytes",
			responseBodies:         []string{advertisement, advertisement + reportStatus},
			wantBodies:             []string{"0000", pktLine("0000000000000000000000000000000000000000 e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") + string(pktline.PktFlush()) + "PACK receive-pack bytes"},
			wantOutput:             advertisement + reportStatus,
			gateAfterFirstByteRead: true,
		},
		{
			desc:           "flush-only push",
			input:          "0000",
			responseBodies: []string{advertisement, advertisement + reportStatus},
			wantBodies:     []string{"0000", "0000"},
			wantOutput:     advertisement + reportStatus,
		},
		{
			desc:           "client closes after advertisement",
			responseBodies: []string{advertisement},
			wantBodies:     []string{"0000"},
			wantOutput:     advertisement,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			secondRequestStarted := make(chan struct{})
			cellServer, captured := startCapturingPushCellServer(t, tc.responseBodies, secondRequestStarted)
			output := newAdvertisementGatedOutput(advertisement)
			var input io.Reader = &advertisementGatedReader{Reader: strings.NewReader(tc.input), advertisementWritten: output.advertisementWritten}
			if tc.gateAfterFirstByteRead {
				input = &firstByteGatedReader{Reader: input, secondRequestStarted: secondRequestStarted}
			}
			command := NewCellsPushCommand(
				cellsTestConfig(t),
				&readwriter.ReadWriter{Out: output, In: input},
				&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion}},
				cellsTestResponse(cellServer.URL),
			)
			result := make(chan error, 1)
			go func() { result <- command.Execute(context.Background()) }()

			select {
			case err := <-result:
				require.NoError(t, err)
			case <-time.After(time.Second):
				require.Fail(t, "Cells push did not complete after the client began sending")
			}

			require.Len(t, *captured, len(tc.wantBodies))
			for index, request := range *captured {
				assert.Equal(t, tc.wantBodies[index], string(request.body))
				assert.Equal(t, "/group/project.git/ssh-receive-pack", request.path)
				assert.NotEmpty(t, request.headers.Get("Gitlab-Shell-Api-Request"))
				assert.Equal(t, testGitProtocolVersion, request.headers.Get("Git-Protocol"))
			}
			assert.Equal(t, tc.wantOutput, output.String())
		})
	}
}

func TestCellsPushReturnsContextErrorWhenCancelled(t *testing.T) {
	advertisement := pktLine("e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\n") + string(pktline.PktFlush())
	cellServer, captured := startCapturingPushCellServer(t, []string{advertisement}, nil)
	output := newAdvertisementGatedOutput(advertisement)
	inputReader, inputWriter := io.Pipe()
	t.Cleanup(func() { inputWriter.Close() })
	command := NewCellsPushCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: output, In: inputReader},
		&commandargs.Shell{},
		cellsTestResponse(cellServer.URL),
	)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- command.Execute(ctx) }()

	select {
	case <-output.advertisementWritten:
	case <-time.After(time.Second):
		require.Fail(t, "Cells push did not relay the advertisement")
	}

	cancel()
	require.NoError(t, inputWriter.Close())

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.Fail(t, "Cells push did not stop after input closed")
	}
	require.Len(t, *captured, 1)
}

func TestCopyAdvertisement(t *testing.T) {
	writerError := errors.New("write failed")
	testCases := []struct {
		desc          string
		input         string
		writer        io.Writer
		wantOutput    string
		wantRemaining string
		wantError     error
		wantErrorText string
	}{
		{desc: "stops at first flush", input: pktLine("refs\n") + "0000", wantOutput: pktLine("refs\n") + "0000"},
		{desc: "leaves bytes after flush unread", input: pktLine("refs\n") + "0000status", wantOutput: pktLine("refs\n") + "0000", wantRemaining: "status"},
		{desc: "EOF before flush", input: pktLine("refs\n"), wantOutput: pktLine("refs\n"), wantError: io.EOF, wantErrorText: "advertisement ended before flush"},
		{desc: "truncated pkt-line", input: "0008abc", wantError: io.ErrUnexpectedEOF},
		{desc: "malformed length prefix", input: "zzzz", wantError: strconv.ErrSyntax, wantErrorText: "decode length"},
		{desc: "writer failure", input: pktLine("refs\n") + "0000", writer: errorWriter{err: writerError}, wantRemaining: "0000", wantError: writerError},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			reader := bytes.NewBufferString(tc.input)
			output := &bytes.Buffer{}
			writer := tc.writer
			if writer == nil {
				writer = output
			}

			err := copyAdvertisement(writer, reader)

			if tc.wantError == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantError)
			}
			if tc.wantErrorText != "" {
				assert.Contains(t, err.Error(), tc.wantErrorText)
			}
			assert.Equal(t, tc.wantOutput, output.String())
			assert.Equal(t, tc.wantRemaining, reader.String())
		})
	}
}

type advertisementGatedReader struct {
	io.Reader
	advertisementWritten <-chan struct{}
}

func (r *advertisementGatedReader) Read(buffer []byte) (int, error) {
	<-r.advertisementWritten

	return r.Reader.Read(buffer)
}

type advertisementGatedOutput struct {
	bytes.Buffer
	advertisement        string
	advertisementWritten chan struct{}
}

func newAdvertisementGatedOutput(advertisement string) *advertisementGatedOutput {
	return &advertisementGatedOutput{
		advertisement:        advertisement,
		advertisementWritten: make(chan struct{}),
	}
}

func (w *advertisementGatedOutput) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if w.Len() >= len(w.advertisement) {
		select {
		case <-w.advertisementWritten:
		default:
			close(w.advertisementWritten)
		}
	}

	return n, err
}

type firstByteGatedReader struct {
	io.Reader
	secondRequestStarted <-chan struct{}
	firstRead            bool
}

func (r *firstByteGatedReader) Read(buffer []byte) (int, error) {
	if !r.firstRead {
		r.firstRead = true
		return r.Reader.Read(buffer[:1])
	}
	<-r.secondRequestStarted

	return r.Reader.Read(buffer)
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestBuildCellsGitClient(t *testing.T) {
	cfg := cellsTestConfig(t)

	t.Run("constructs correct URL from CellAddress and GlProjectPath", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com")
		args := &commandargs.Shell{}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.Equal(t, "http://cell1.example.com/group/project.git", gitClient.URL)
	})

	t.Run("strips trailing slash from CellAddress", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com/")
		args := &commandargs.Shell{}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.Equal(t, "http://cell1.example.com/group/project.git", gitClient.URL)
	})

	t.Run("preserves path component from CellAddress", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com/gitlab")
		args := &commandargs.Shell{}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.Equal(t, "http://cell1.example.com/gitlab/group/project.git", gitClient.URL)
	})

	t.Run("returns error when GlProjectPath is empty", func(t *testing.T) {
		response := &accessverifier.Response{
			CellAddress: "http://cell1.example.com",
			Who:         "key-123",
			Gitaly: accessverifier.Gitaly{
				Repo: pb.Repository{},
			},
		}
		args := &commandargs.Shell{}

		_, err := buildCellsGitClient(cfg, response, args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing gl_project_path")
	})

	t.Run("sets Gitlab-Shell-Api-Request header with valid JWT", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com")
		args := &commandargs.Shell{}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.NotContains(t, gitClient.Headers, shellJWTHeaderName)
		require.NotNil(t, gitClient.HeaderFunc)

		headers, err := gitClient.HeaderFunc()
		require.NoError(t, err)
		tokenString := headers[shellJWTHeaderName]
		require.NotEmpty(t, tokenString)

		claims := &clientpkg.ShellClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
			return []byte(testSecret), nil
		})
		require.NoError(t, err)
		require.True(t, token.Valid)
		require.Equal(t, "gitlab-shell", claims.Issuer)
		require.Equal(t, "user-1", claims.GlID)
	})

	t.Run("returns error when CellAddress has no scheme", func(t *testing.T) {
		response := cellsTestResponse("cell1.example.com")
		args := &commandargs.Shell{}

		_, err := buildCellsGitClient(cfg, response, args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing URL scheme")
	})

	t.Run("sets Git-Protocol header when GitProtocolVersion is set", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com")
		args := &commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion}}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.Equal(t, testGitProtocolVersion, gitClient.Headers["Git-Protocol"])
	})

	t.Run("omits Git-Protocol header when GitProtocolVersion is empty", func(t *testing.T) {
		response := cellsTestResponse("http://cell1.example.com")
		args := &commandargs.Shell{}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		_, hasGitProtocol := gitClient.Headers["Git-Protocol"]
		require.False(t, hasGitProtocol)
	})
}

// stubShellJWTSigner replaces signShellJWT with a counter so tests can assert
// that each request carries a token signed when that request started.
func stubShellJWTSigner(t *testing.T) *int {
	t.Helper()
	calls := 0
	original := signShellJWT
	signShellJWT = func(_, _ string) (string, error) {
		calls++
		return fmt.Sprintf("jwt-%d", calls), nil
	}
	t.Cleanup(func() { signShellJWT = original })

	return &calls
}

func TestCellsPushSignsShellJWTPerRequest(t *testing.T) {
	calls := stubShellJWTSigner(t)
	advertisement := pktLine("e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") + string(pktline.PktFlush())
	reportStatus := pktLine("unpack ok\n") + string(pktline.PktFlush())
	cellServer, captured := startCapturingPushCellServer(t, []string{advertisement, advertisement + reportStatus}, nil)
	output := newAdvertisementGatedOutput(advertisement)
	input := &advertisementGatedReader{
		Reader:               strings.NewReader(pktLine("0000000000000000000000000000000000000000 e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") + string(pktline.PktFlush()) + "PACK"),
		advertisementWritten: output.advertisementWritten,
	}

	err := NewCellsPushCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: output, In: input},
		&commandargs.Shell{},
		cellsTestResponse(cellServer.URL),
	).Execute(context.Background())

	require.NoError(t, err)
	require.Len(t, *captured, 2)
	assert.Equal(t, 2, *calls)
	assert.Equal(t, "jwt-1", (*captured)[0].headers.Get(shellJWTHeaderName))
	assert.Equal(t, "jwt-2", (*captured)[1].headers.Get(shellJWTHeaderName))
}

func TestCellsPullSignsShellJWTAtRequestTime(t *testing.T) {
	calls := stubShellJWTSigner(t)
	cellServer, captured := startCapturingCellServer(t, "cell-response")

	command := NewCellsPullCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: io.Discard, In: strings.NewReader(fetchV2Request)},
		&commandargs.Shell{},
		cellsTestResponse(cellServer.URL),
	)
	assert.Equal(t, 0, *calls, "JWT must not be signed before the request starts")

	require.NoError(t, command.Execute(context.Background()))
	assert.Equal(t, 1, *calls)
	assert.Equal(t, "jwt-1", captured.headers.Get(shellJWTHeaderName))
}

func TestCellsCommandsReturnShellJWTSigningError(t *testing.T) {
	signErr := errors.New("signing failed")
	original := signShellJWT
	signShellJWT = func(_, _ string) (string, error) { return "", signErr }
	t.Cleanup(func() { signShellJWT = original })

	cellServer, captured := startCapturingPushCellServer(t, nil, nil)

	err := NewCellsPushCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: io.Discard, In: strings.NewReader("0000")},
		&commandargs.Shell{},
		cellsTestResponse(cellServer.URL),
	).Execute(context.Background())
	require.ErrorIs(t, err, signErr)
	require.Contains(t, err.Error(), "generating Shell JWT")

	err = NewCellsPullCommand(
		cellsTestConfig(t),
		&readwriter.ReadWriter{Out: io.Discard, In: strings.NewReader(fetchV2Request)},
		&commandargs.Shell{},
		cellsTestResponse(cellServer.URL),
	).Execute(context.Background())
	require.ErrorIs(t, err, signErr)
	require.Empty(t, *captured)
}

type capturedRequest struct {
	path    string
	headers http.Header
	body    []byte
}

func startCapturingPushCellServer(t *testing.T, responseBodies []string, secondRequestStarted chan struct{}) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	captured := &[]capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secondRequestStarted != nil && len(*captured) == 1 {
			close(secondRequestStarted)
		}
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		*captured = append(*captured, capturedRequest{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		requestIndex := len(*captured) - 1
		if !assert.Less(t, requestIndex, len(responseBodies)) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(responseBodies[requestIndex]))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	return server, captured
}

func startCapturingCellServer(t *testing.T, responseBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.headers = r.Header
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		captured.body = body
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(responseBody))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func cellsTestConfig(t *testing.T) *config.Config {
	t.Helper()

	return &config.Config{
		Secret: testSecret,
	}
}

func cellsTestResponse(cellAddress string) *accessverifier.Response {
	return &accessverifier.Response{
		Success:     true,
		UserID:      "user-1",
		Username:    "alex-doe",
		CellAddress: cellAddress,
		Who:         "key-123",
		Gitaly: accessverifier.Gitaly{
			Repo: pb.Repository{
				StorageName:   "storage_name",
				RelativePath:  "relative_path",
				GlRepository:  "project-1",
				GlProjectPath: "group/project",
			},
			Address: "unix:///fake/gitaly.sock",
			Token:   testGitalyToken,
		},
	}
}
