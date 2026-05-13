package githttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"

	clientpkg "gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

const testSecret = "test-secret-for-cells"

func TestCellsCommandExecute(t *testing.T) {
	testCases := []struct {
		desc         string
		expectedPath string
		responseBody string
		execute      func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error
	}{
		{
			desc:         "pull uses ssh-upload-pack",
			expectedPath: "/group/project.git/ssh-upload-pack",
			responseBody: "upload-pack-cell-response",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return (&CellsPullCommand{Config: cfg, ReadWriter: rw, Args: args, Response: resp}).Execute(ctx)
			},
		},
		{
			desc:         "push uses ssh-receive-pack",
			expectedPath: "/group/project.git/ssh-receive-pack",
			responseBody: "receive-pack-cell-response",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return (&CellsPushCommand{Config: cfg, ReadWriter: rw, Args: args, Response: resp}).Execute(ctx)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cellServer := startMockCellServer(t, tc.expectedPath, tc.responseBody)
			cfg := cellsTestConfig(t)

			output := &bytes.Buffer{}
			input := strings.NewReader("input-data")

			err := tc.execute(
				context.Background(),
				cfg,
				&readwriter.ReadWriter{Out: output, In: input},
				&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: "version=2"}},
				cellsTestResponse(cellServer.URL),
			)

			require.NoError(t, err)
			require.Equal(t, tc.responseBody, output.String())
		})
	}
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
		args := &commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: "version=2"}}

		gitClient, err := buildCellsGitClient(cfg, response, args)
		require.NoError(t, err)
		require.Equal(t, "version=2", gitClient.Headers["Git-Protocol"])
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

func TestCellsCommandVerifiesJWTAndHeaders(t *testing.T) {
	testCases := []struct {
		desc         string
		expectedPath string
		execute      func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error
	}{
		{
			desc:         "pull sends correct path and headers",
			expectedPath: "/group/project.git/ssh-upload-pack",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return (&CellsPullCommand{Config: cfg, ReadWriter: rw, Args: args, Response: resp}).Execute(ctx)
			},
		},
		{
			desc:         "push sends correct path and headers",
			expectedPath: "/group/project.git/ssh-receive-pack",
			execute: func(ctx context.Context, cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) error {
				return (&CellsPushCommand{Config: cfg, ReadWriter: rw, Args: args, Response: resp}).Execute(ctx)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var receivedHeaders http.Header
			var receivedPath string
			var receivedBody []byte

			cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header
				receivedPath = r.URL.Path
				var err error
				receivedBody, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("response"))
			}))
			t.Cleanup(cellServer.Close)

			cfg := cellsTestConfig(t)
			inputData := "test-stdin-data"

			err := tc.execute(
				context.Background(),
				cfg,
				&readwriter.ReadWriter{Out: &bytes.Buffer{}, In: strings.NewReader(inputData)},
				&commandargs.Shell{Env: sshenv.Env{GitProtocolVersion: "version=2"}},
				cellsTestResponse(cellServer.URL),
			)

			require.NoError(t, err)
			require.Equal(t, tc.expectedPath, receivedPath)
			require.NotEmpty(t, receivedHeaders.Get("Gitlab-Shell-Api-Request"))
			require.Equal(t, "version=2", receivedHeaders.Get("Git-Protocol"))
			require.Equal(t, inputData, string(receivedBody))
		})
	}
}

func startMockCellServer(t *testing.T, expectedPath, responseBody string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, expectedPath, r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Gitlab-Shell-Api-Request"))
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(responseBody))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
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
			Token:   "token",
		},
	}
}
