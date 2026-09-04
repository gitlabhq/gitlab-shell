package githttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	pushRequest := pktLine("0000000000000000000000000000000000000000 e56497bb5f03a90a51293fc6d516788730953899 refs/heads/main\x00report-status\n") +
		string(pktline.PktFlush()) + "PACK receive-pack bytes"

	testCases := []struct {
		desc         string
		input        string
		expectedBody string
		expectedPath string
		execute      func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error
	}{
		{
			desc:         "protocol v2 fetch gets exactly one trailing flush",
			input:        fetchV2Request,
			expectedBody: fetchV2Request,
			expectedPath: "/group/project.git/ssh-upload-pack",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return NewCellsPullCommand(cfg, rw, args, resp).Execute(ctx)
			},
		},
		{
			desc:         "protocol v2 ls-refs without done forwards through EOF",
			input:        lsRefsV2Request,
			expectedBody: lsRefsV2Request,
			expectedPath: "/group/project.git/ssh-upload-pack",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return NewCellsPullCommand(cfg, rw, args, resp).Execute(ctx)
			},
		},
		{
			desc:         "push remains raw streaming",
			input:        pushRequest,
			expectedBody: pushRequest,
			expectedPath: "/group/project.git/ssh-receive-pack",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return NewCellsPushCommand(cfg, rw, args, resp).Execute(ctx)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cellServer, captured := startCapturingCellServer(t, responseBody)
			cfg := cellsTestConfig(t)

			output := &bytes.Buffer{}
			input := strings.NewReader(tc.input)

			err := tc.execute(
				context.Background(),
				cfg,
				&readwriter.ReadWriter{Out: output, In: input},
				&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion}},
				cellsTestResponse(cellServer.URL),
			)

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

		tokenString := gitClient.Headers["Gitlab-Shell-Api-Request"]
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

type capturedRequest struct {
	path    string
	headers http.Header
	body    []byte
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
