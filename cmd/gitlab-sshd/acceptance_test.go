package main_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mikesmitty/edkey"
	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gitalyClient "gitlab.com/gitlab-org/gitaly/v18/client"
	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/v18/streamio"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

var (
	sshdPath       = ""
	gitalyConnInfo *gitalyConnectionInfo
	gitalyRepoOnce sync.Once
	gitalyRepoErr  error
	keyTypes       = []string{
		"rsa-2048",
		"rsa-4096",
		"ed25519",
		"ecdsa-p256",
		"ecdsa-p384",
		"ecdsa-p521",
	}
)

const (
	testRepo              = "test-gitlab-shell/gitlab-test.git"
	gitalyInfoRefsTimeout = 10 * time.Second

	// Bound local seeding so setup fits within the package's 1m test timeout.
	gitalySeedTimeout   = 15 * time.Second
	gitalyRemoveTimeout = 2 * time.Second
)

type gitalyConnectionInfo struct {
	Address string `json:"address"`
	Storage string `json:"storage"`
}

func generateKey(keyType string) (interface{}, error) {
	switch strings.ToLower(keyType) {
	case "rsa-2048":
		return rsa.GenerateKey(rand.Reader, 2048)
	case "rsa-4096":
		return rsa.GenerateKey(rand.Reader, 4096)
	case "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	case "ecdsa-p256":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ecdsa-p384":
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "ecdsa-p521":
		return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", keyType)
	}
}

func init() {
	rootDir := rootDir()
	sshdPath = filepath.Join(rootDir, "bin", "gitlab-sshd")

	if _, err := os.Stat(sshdPath); os.IsNotExist(err) {
		panic(fmt.Errorf("cannot find executable %s. Please run 'make compile'", sshdPath))
	}

	gci, exists := os.LookupEnv("GITALY_CONNECTION_INFO")
	if exists {
		json.Unmarshal([]byte(gci), &gitalyConnInfo)
	}
}

func rootDir() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		panic(fmt.Errorf("rootDir: calling runtime.Caller failed"))
	}

	return filepath.Join(filepath.Dir(currentFile), "..", "..")
}

func gitEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=GitLab Shell",
		"GIT_AUTHOR_EMAIL=gitlab-shell@example.com",
		"GIT_COMMITTER_NAME=GitLab Shell",
		"GIT_COMMITTER_EMAIL=gitlab-shell@example.com",
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
	)
}

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, output)
	}
	return output, nil
}

func createTestRepoBundle(ctx context.Context, dir string) (string, error) {
	if _, err := runGit(ctx, dir, "init", "--quiet", "--object-format=sha1", "--initial-branch=main", "src"); err != nil {
		return "", err
	}
	srcDir := filepath.Join(dir, "src")
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("GitLab Shell acceptance test repository\n"), 0644); err != nil {
		return "", fmt.Errorf("write README.md: %w", err)
	}
	bundlePath := filepath.Join(dir, "seed.bundle")
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "--quiet", "-m", "Initial commit"},
		{"tag", "v1.0.0"},
		{"bundle", "create", "--quiet", bundlePath, "--all"},
	} {
		if _, err := runGit(ctx, srcDir, args...); err != nil {
			return "", err
		}
	}
	return bundlePath, nil
}

func TestCreateTestRepoBundle(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "commit.gpgsign")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "nonexistent"))

	dir := t.TempDir()
	bundlePath, err := createTestRepoBundle(t.Context(), dir)
	require.NoError(t, err)

	output, err := runGit(t.Context(), dir, "bundle", "list-heads", bundlePath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, lines, 3)
	for _, ref := range []string{"HEAD", "refs/heads/main", "refs/tags/v1.0.0"} {
		require.Regexp(t, `(?m)^[0-9a-f]{40} `+regexp.QuoteMeta(ref)+`$`, string(output))
	}
}

func ensureGitalyRepository(t *testing.T) *pb.Repository {
	if os.Getenv("GITALY_CONNECTION_INFO") == "" {
		t.Skip("GITALY_CONNECTION_INFO is not set")
	}
	require.NotNil(t, gitalyConnInfo)

	glRepository := &pb.Repository{StorageName: gitalyConnInfo.Storage, RelativePath: testRepo}

	// Tests share one seeded repo per package and must treat it as read-only.
	gitalyRepoOnce.Do(func() { gitalyRepoErr = seedGitalyRepository(gitalyConnInfo.Address, glRepository) })
	require.NoError(t, gitalyRepoErr)

	return glRepository
}

func seedGitalyRepository(address string, glRepository *pb.Repository) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitalySeedTimeout)
	defer cancel()

	defer func() {
		if err != nil {
			err = fmt.Errorf("seed gitaly repository from bundle: %w", err)
		}
	}()

	dir, err := os.MkdirTemp("", "gitlab-shell-gitaly-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bundlePath, err := createTestRepoBundle(ctx, dir)
	if err != nil {
		return err
	}
	bundle, err := os.Open(bundlePath)
	if err != nil {
		return err
	}
	defer bundle.Close()

	conn, err := gitalyClient.Dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()

	repository := pb.NewRepositoryServiceClient(conn)
	removeCtx, removeCancel := context.WithTimeout(ctx, gitalyRemoveTimeout)
	_, _ = repository.RemoveRepository(removeCtx, &pb.RemoveRepositoryRequest{Repository: glRepository})
	removeCancel()

	return uploadBundle(ctx, repository, glRepository, bundle)
}

func uploadBundle(ctx context.Context, client pb.RepositoryServiceClient, repo *pb.Repository, bundle io.Reader) error {
	stream, err := client.CreateRepositoryFromBundle(ctx)
	if err != nil {
		return err
	}
	sendErr := func() error {
		if err := stream.Send(&pb.CreateRepositoryFromBundleRequest{Repository: repo}); err != nil {
			return err
		}
		_, err := io.Copy(streamio.NewWriter(func(p []byte) error {
			return stream.Send(&pb.CreateRepositoryFromBundleRequest{Data: p})
		}), bundle)
		return err
	}()
	// Always close via CloseAndRecv: the server's status takes precedence over local send/copy errors,
	// which are often just io.EOF from a server that already failed.
	if _, recvErr := stream.CloseAndRecv(); recvErr != nil {
		return recvErr
	}
	return sendErr
}

type rejectingRepositoryServer struct {
	pb.UnimplementedRepositoryServiceServer
	err error
}

func (s *rejectingRepositoryServer) CreateRepositoryFromBundle(pb.RepositoryService_CreateRepositoryFromBundleServer) error {
	return s.err
}

func TestUploadBundleRejection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	pb.RegisterRepositoryServiceServer(server, &rejectingRepositoryServer{err: status.Error(codes.AlreadyExists, "bundle rejected")})
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	conn, err := gitalyClient.Dial("tcp://" + listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	err = uploadBundle(t.Context(), pb.NewRepositoryServiceClient(conn), &pb.Repository{StorageName: "default", RelativePath: testRepo}, strings.NewReader("not a bundle"))
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	require.ErrorContains(t, err, "bundle rejected")
}

func startGitOverHTTPServer(t *testing.T) string {
	glRepository := ensureGitalyRepository(t)
	require.NotNil(t, gitalyConnInfo)
	conn, err := gitalyClient.Dial(gitalyConnInfo.Address)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	client := pb.NewSmartHTTPServiceClient(conn)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/info/refs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				rpcRequest := &pb.InfoRefsRequest{
					Repository: glRepository,
				}

				var reader io.Reader
				switch r.URL.Query().Get("service") {
				case "git-receive-pack":
					ctx, cancel := context.WithTimeout(r.Context(), gitalyInfoRefsTimeout)
					defer cancel()
					stream, err := client.InfoRefsReceivePack(ctx, rpcRequest)
					assert.NoError(t, err)
					reader = streamio.NewReader(func() ([]byte, error) {
						resp, err := stream.Recv()
						return resp.GetData(), err
					})
				default:
					t.FailNow()
				}

				_, err := io.Copy(w, reader)
				assert.NoError(t, err)
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(_ http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				assert.Equal(t, "0000", string(body))
			},
		},
	}

	return testserver.StartHTTPServer(t, requests)
}

func buildAllowedResponse(t *testing.T, filename string) string {
	testRoot := testhelper.PrepareTestRootDir(t)

	body, err := os.ReadFile(filepath.Join(testRoot, filename))
	require.NoError(t, err)

	response := strings.Replace(string(body), "GITALY_REPOSITORY", testRepo, 1)

	if gitalyConnInfo != nil {
		response = strings.Replace(response, "GITALY_ADDRESS", gitalyConnInfo.Address, 1)
		response = strings.Replace(response, "GITALY_STORAGE", gitalyConnInfo.Storage, 1)
	}

	return response
}

type customHandler struct {
	url    string
	caller http.HandlerFunc
}

func successAPI(t *testing.T, handlers ...customHandler) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("gitlab-api-mock: received request: %s %s", r.Method, r.RequestURI)
		w.Header().Set("Content-Type", "application/json")

		url := r.URL.EscapedPath()

		for _, handler := range handlers {
			if url == handler.url {
				handler.caller(w, r)

				return
			}
		}

		switch url {
		case "/api/v4/internal/authorized_keys":
			fmt.Fprintf(w, `{"id":1, "key":"%s"}`, r.FormValue("key"))
		case "/api/v4/internal/allowed":
			response := buildAllowedResponse(t, "responses/allowed_without_console_messages.json")

			_, err := fmt.Fprint(w, response)
			assert.NoError(t, err)
		case "/api/v4/internal/shellhorse/git_audit_event":
			w.WriteHeader(http.StatusOK)
			return
		default:
			t.Logf("Unexpected request to successAPI: %s", r.URL.EscapedPath())
			t.FailNow()
		}
	})
}

func genServerConfig(gitlabURL, hostKeyPath string) []byte {
	return []byte(`---
user: "git"
log_file: ""
log_format: json
secret: "0123456789abcdef"
gitlab_url: "` + gitlabURL + `"
sshd:
  listen: "127.0.0.1:0"
  proxy_protocol: true
  web_listen: ""
  host_key_files:
    - "` + hostKeyPath + `"`)
}

func buildClient(t *testing.T, addr string, clientKeyType string, hostKey ed25519.PublicKey) *ssh.Client {
	t.Helper()

	pubKey, err := ssh.NewPublicKey(hostKey)
	require.NoError(t, err)

	var clientPrivKey interface{}

	clientPrivKey, err = generateKey(clientKeyType)
	require.NoError(t, err)

	clientSigner, err := ssh.NewSignerFromKey(clientPrivKey)
	require.NoError(t, err)

	// Use the proxy protocol to spoof our client address
	target, err := net.ResolveTCPAddr("tcp", addr)
	require.NoError(t, err)
	conn, err := net.DialTCP("tcp", nil, target)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	// Create a proxyprotocol header or use HeaderProxyFromAddrs() if you
	// have two conn's
	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   net.ParseIP("10.1.1.1"),
			Port: 1000,
		},
		DestinationAddr: target,
	}
	// After the connection was created write the proxy headers first
	_, err = header.WriteTo(conn)
	require.NoError(t, err)

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.FixedHostKey(pubKey),
	})
	require.NoError(t, err)

	client := ssh.NewClient(sshConn, chans, reqs)
	t.Cleanup(func() { client.Close() })

	return client
}

func configureSSHD(t *testing.T, apiServer string) (string, ed25519.PublicKey) {
	t.Helper()

	tempDir := t.TempDir()

	configFile := filepath.Join(tempDir, "config.yml")
	hostKeyFile := filepath.Join(tempDir, "hostkey")

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	configFileData := genServerConfig(apiServer, hostKeyFile)
	require.NoError(t, os.WriteFile(configFile, configFileData, 0644))

	block := &pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: edkey.MarshalED25519PrivateKey(priv)}
	hostKeyData := pem.EncodeToMemory(block)
	require.NoError(t, os.WriteFile(hostKeyFile, hostKeyData, 0400))

	return tempDir, pub
}

func startSSHD(t *testing.T, dir string) string {
	t.Helper()

	// We need to scan the first few lines of stderr to get the listen address.
	// Once we've learned it, we'll start a goroutine to copy everything to
	// the real stderr
	pr, pw := io.Pipe()
	t.Cleanup(func() { pr.Close() })
	t.Cleanup(func() { pw.Close() })

	scanner := bufio.NewScanner(pr)
	extractor := regexp.MustCompile(`tcp_address"?[=:]"?([0-9a-f\[\]\.:]+)"?`)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, sshdPath, "-config-dir", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = pw
	require.NoError(t, cmd.Start())
	t.Logf("gitlab-sshd: Start(): success")
	t.Cleanup(func() { t.Logf("gitlab-sshd: Wait(): %v", cmd.Wait()) })
	t.Cleanup(cancel)

	var listenAddr string
	for scanner.Scan() {
		if matches := extractor.FindSubmatch(scanner.Bytes()); len(matches) == 2 {
			listenAddr = string(matches[1])
			break
		}
	}
	require.NotEmpty(t, listenAddr, "Couldn't extract listen address from gitlab-sshd")

	go io.Copy(os.Stderr, pr)

	return listenAddr
}

// Starts an instance of gitlab-sshd with the given arguments, returning an SSH
// client already connected to it
func runSSHD(t *testing.T, clientKeyType string, apiHandler http.Handler) *ssh.Client {
	t.Helper()

	// Set up a stub gitlab server
	apiServer := httptest.NewServer(apiHandler)
	t.Logf("gitlab-api-mock: started: url=%q", apiServer.URL)
	t.Cleanup(func() {
		apiServer.Close()
		t.Logf("gitlab-api-mock: closed")
	})

	dir, hostKey := configureSSHD(t, apiServer.URL)
	listenAddr := startSSHD(t, dir)

	return buildClient(t, listenAddr, clientKeyType, hostKey)
}

func TestDiscoverSuccess(t *testing.T) {
	handler := customHandler{
		url: "/api/v4/internal/discover",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"id": 1000, "name": "Test User", "username": "test-user"}`)
		},
	}
	client := runSSHD(t, "ed25519", successAPI(t, handler))

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()

	output, err := session.Output("discover")
	require.NoError(t, err)
	require.Equal(t, "Welcome to GitLab, @test-user!\n", string(output))
}

func TestPersonalAccessTokenSuccess(t *testing.T) {
	handler := customHandler{
		url: "/api/v4/internal/personal_access_token",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"success": true, "token": "testtoken", "scopes": ["api"], "expires_at": "9001-01-01"}`)
		},
	}
	client := runSSHD(t, "ed25519", successAPI(t, handler))

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()

	output, err := session.Output("personal_access_token test api")
	require.NoError(t, err)
	require.Equal(t, "Token:   testtoken\nScopes:  api\nExpires: 9001-01-01\n", string(output))
}

func TestTwoFactorAuthRecoveryCodesSuccess(t *testing.T) {
	handler := customHandler{
		url: "/api/v4/internal/two_factor_recovery_codes",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"success": true, "recovery_codes": ["code1", "code2"]}`)
		},
	}
	client := runSSHD(t, "ed25519", successAPI(t, handler))
	session, stdin, stdout := newSession(t, client)

	reader := bufio.NewReader(stdout)

	err := session.Start("2fa_recovery_codes")
	require.NoError(t, err)

	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "Are you sure you want to generate new two-factor recovery codes?\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "Any existing recovery codes you saved will be invalidated. (yes/no)\n", line)

	_, err = fmt.Fprintln(stdin, "yes")
	require.NoError(t, err)

	output, err := io.ReadAll(stdout)
	require.NoError(t, err)
	require.Equal(t, `
Your two-factor authentication recovery codes are:

code1
code2

During sign in, use one of the codes above when prompted for
your two-factor code. Then, visit your Profile Settings and add
a new device so you do not lose access to your account again.
`, string(output))
}

func TwoFactorAuthVerifySuccess(t *testing.T) {
	handler := customHandler{
		url: "/api/v4/internal/two_factor_otp_check",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"success": true}`)
		},
	}
	client := runSSHD(t, "ed25519", successAPI(t, handler))
	session, stdin, stdout := newSession(t, client)

	reader := bufio.NewReader(stdout)

	err := session.Start("2fa_verify")
	require.NoError(t, err)

	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "OTP: ", line)

	_, err = fmt.Fprintln(stdin, "otp123")
	require.NoError(t, err)

	output, err := io.ReadAll(stdout)
	require.NoError(t, err)
	require.Equal(t, "OTP validation successful. Git operations are now allowed.\n", string(output))
}

func TestGitLfsAuthenticateSuccess(t *testing.T) {
	t.Run("lfs is successfully authed when a correct user is provided", func(t *testing.T) {
		handler := customHandler{
			url: "/api/v4/internal/lfs_authenticate",
			caller: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, `{"username": "test-user", "lfs_token": "testlfstoken", "repo_path": "foo", "expires_in": 7200}`)
			},
		}
		client := runSSHD(t, "ed25519", successAPI(t, handler))

		session, err := client.NewSession()
		require.NoError(t, err)
		defer session.Close()

		output, err := session.Output("git-lfs-authenticate test-user/repo.git download")

		require.NoError(t, err)
		require.JSONEq(t, `{"header":{"Authorization":"Basic dGVzdC11c2VyOnRlc3RsZnN0b2tlbg=="},"href":"/info/lfs","expires_in":7200}
`, string(output))
	})

	t.Run("lfs is not authenticated when a user is not allowed to perform an action", func(t *testing.T) {
		handler := customHandler{
			url: "/api/v4/internal/lfs_authenticate",
			caller: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			},
		}
		client := runSSHD(t, "ed25519", successAPI(t, handler))
		session, err := client.NewSession()
		require.NoError(t, err)
		defer session.Close()

		output, err := session.Output("git-lfs-authenticate test-user/repo.git download")

		// we don't send back an error
		require.NoError(t, err)
		// we also ensure that we don't send back any output
		require.Empty(t, string(output))
	})
}

func TestGitReceivePackSuccess(t *testing.T) {
	ensureGitalyRepository(t)

	for _, keyType := range keyTypes {
		t.Run(keyType, func(t *testing.T) {
			client := runSSHD(t, keyType, successAPI(t))
			session, stdin, stdout := newSession(t, client)

			err := session.Start(fmt.Sprintf("git-receive-pack %s", testRepo))
			require.NoError(t, err)

			// Gracefully close connection
			_, err = fmt.Fprintln(stdin, "0000")
			require.NoError(t, err)
			stdin.Close()

			output, err := io.ReadAll(stdout)
			require.NoError(t, err)

			outputLines := strings.Split(string(output), "\n")
			for i := 0; i < (len(outputLines) - 1); i++ {
				require.Regexp(t, "^[0-9a-f]{44} refs/(heads|tags)/[^ ]+", outputLines[i])
			}
			require.Equal(t, "0000", outputLines[len(outputLines)-1])
		})
	}
}

func TestGeoGitReceivePackSuccess(t *testing.T) {
	url := startGitOverHTTPServer(t)

	handler := customHandler{
		url: "/api/v4/internal/allowed",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			response := buildAllowedResponse(t, "responses/allowed_with_geo_push_payload.json")
			response = strings.Replace(response, "PRIMARY_REPO", url, 1)

			w.WriteHeader(300)
			_, err := fmt.Fprint(w, response)
			assert.NoError(t, err)
		},
	}
	client := runSSHD(t, "ed25519", successAPI(t, handler))
	session, stdin, stdout := newSession(t, client)

	err := session.Start(fmt.Sprintf("git-receive-pack %s", testRepo))
	require.NoError(t, err)

	// Gracefully close connection
	_, err = fmt.Fprintln(stdin, "0000")
	require.NoError(t, err)
	stdin.Close()

	output, err := io.ReadAll(stdout)
	require.NoError(t, err)

	outputLines := strings.Split(string(output), "\n")

	for i := 0; i < (len(outputLines) - 1); i++ {
		require.Regexp(t, "^[0-9a-f]{44} refs/(heads|tags)/[^ ]+", outputLines[i])
	}

	require.Equal(t, "0000", outputLines[len(outputLines)-1])
}

func TestGitUploadPackSuccess(t *testing.T) {
	ensureGitalyRepository(t)

	client := runSSHD(t, "ed25519", successAPI(t))
	defer client.Close()

	numberOfSessions := 3
	for sessionNumber := 0; sessionNumber < numberOfSessions; sessionNumber++ {
		t.Run(fmt.Sprintf("session #%v", sessionNumber), func(t *testing.T) {
			session, stdin, stdout := newSession(t, client)
			reader := bufio.NewReader(stdout)

			err := session.Start(fmt.Sprintf("git-upload-pack %s", testRepo))
			require.NoError(t, err)

			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Regexp(t, "^[0-9a-f]{44} HEAD.+", line)

			// Gracefully close connection
			_, err = fmt.Fprintln(stdin, "0000")
			require.NoError(t, err)

			output, err := io.ReadAll(reader)
			require.NoError(t, err)

			outputLines := strings.Split(string(output), "\n")

			require.Len(t, outputLines, 3)
			require.Regexp(t, `^[0-9a-f]{44} refs/heads/main$`, outputLines[0])
			require.Regexp(t, `^[0-9a-f]{44} refs/tags/v1\.0\.0$`, outputLines[1])
			require.Equal(t, "0000", outputLines[2])
		})
	}
}

func TestGitUploadArchiveSuccess(t *testing.T) {
	ensureGitalyRepository(t)

	client := runSSHD(t, "ed25519", successAPI(t))
	session, stdin, stdout := newSession(t, client)
	reader := bufio.NewReader(stdout)

	err := session.Start(fmt.Sprintf("git-upload-archive %s", testRepo))
	require.NoError(t, err)

	// git-upload-archive stops reading at this flush; writing another flush
	// races with the channel closing.
	_, err = fmt.Fprint(stdin, "0012argument HEAD\n0000")
	require.NoError(t, err)

	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "0008ACK\n", line)

	output, err := io.ReadAll(reader)
	require.NoError(t, err)

	require.Truef(t, bytes.HasSuffix(output, []byte("0000")), "unexpected archive output tail: %q", output[max(0, len(output)-64):])
}

func TestStartupFailureExitsWithoutPanic(t *testing.T) {
	tmpDir := t.TempDir()
	missingHostKey := filepath.Join(tmpDir, "nonexistent-host-key")

	testCases := []struct {
		desc      string
		config    string
		wantInLog string
	}{
		{
			// Malformed YAML makes config.NewFromDir fail before ConfigureLogger runs.
			desc:      "invalid config",
			config:    "gitlab_url: http://localhost:3000\nsecret: \"test-secret\"\nsshd: [this is not a mapping\n",
			wantInLog: "failed to load configuration from specified directory",
		},
		{
			// Config passes isConfigSane but its only host key file is missing, so
			// sshd.NewServer fails and startup must abort instead of using a nil server.
			desc: "server startup failure",
			config: "gitlab_url: http://localhost:3000\n" +
				"secret: \"test-secret\"\n" +
				"log_file: \"\"\n" +
				"log_format: json\n" +
				"sshd:\n" +
				"  listen: \"127.0.0.1:0\"\n" +
				"  host_key_files:\n" +
				"    - \"" + missingHostKey + "\"\n",
			wantInLog: "Failed to start Gitlab built-in sshd",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			configDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yml"), []byte(tc.config), 0644))

			output, err := exec.Command(sshdPath, "-config-dir", configDir).CombinedOutput()

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "expected gitlab-sshd to exit non-zero")
			require.Equal(t, 1, exitErr.ExitCode())

			combined := string(output)
			require.Contains(t, combined, tc.wantInLog)
			require.NotContains(t, combined, "panic:")
			require.NotContains(t, combined, "SIGSEGV")
			require.NotContains(t, combined, "invalid memory address")
		})
	}
}

func newSession(t *testing.T, client *ssh.Client) (*ssh.Session, io.WriteCloser, io.Reader) {
	session, err := client.NewSession()
	require.NoError(t, err)

	stdin, err := session.StdinPipe()
	require.NoError(t, err)

	stdout, err := session.StdoutPipe()
	require.NoError(t, err)

	t.Cleanup(func() {
		session.Close()
	})

	return session, stdin, stdout
}
