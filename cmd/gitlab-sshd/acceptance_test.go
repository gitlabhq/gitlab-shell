package main_test

import (
	"bufio"
	"context"
	"crypto/ed25519"
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
	"testing"

	"github.com/mikesmitty/edkey"
	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"
	gitalyClient "gitlab.com/gitlab-org/gitaly/v16/client"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/v16/streamio"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

var (
	sshdPath       = ""
	gitalyConnInfo *gitalyConnectionInfo
)

const (
	testRepo          = "test-gitlab-shell/gitlab-test.git"
	testRepoNamespace = "test-gitlab-shell"
	testRepoImportUrl = "https://gitlab.com/gitlab-org/gitlab-test.git"
)

type gitalyConnectionInfo struct {
	Address string `json:"address"`
	Storage string `json:"storage"`
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

func ensureGitalyRepository(t *testing.T) (*grpc.ClientConn, *pb.Repository) {
	if os.Getenv("GITALY_CONNECTION_INFO") == "" {
		t.Skip("GITALY_CONNECTION_INFO is not set")
	}

	conn, err := gitalyClient.Dial(gitalyConnInfo.Address, gitalyClient.DefaultDialOpts)
	require.NoError(t, err)

	repository := pb.NewRepositoryServiceClient(conn)

	glRepository := &pb.Repository{StorageName: gitalyConnInfo.Storage, RelativePath: testRepo}

	// Remove the test repository before running the tests
	removeReq := &pb.RemoveRepositoryRequest{Repository: glRepository}
	// Ignore the error because the repository may not exist
	repository.RemoveRepository(context.Background(), removeReq)

	createReq := &pb.CreateRepositoryFromURLRequest{Repository: glRepository, Url: testRepoImportUrl}
	_, err = repository.CreateRepositoryFromURL(context.Background(), createReq)
	require.NoError(t, err)

	return conn, glRepository
}

func startGitOverHTTPServer(t *testing.T) string {
	ctx := context.Background()
	conn, glRepository := ensureGitalyRepository(t)
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
					stream, err := client.InfoRefsReceivePack(ctx, rpcRequest)
					require.NoError(t, err)
					reader = streamio.NewReader(func() ([]byte, error) {
						resp, err := stream.Recv()
						return resp.GetData(), err
					})
				default:
					t.FailNow()
				}

				_, err := io.Copy(w, reader)
				require.NoError(t, err)
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				require.Equal(t, string(body), "0000")
			},
		},
	}

	return testserver.StartHttpServer(t, requests)
}

func buildAllowedResponse(t *testing.T, filename string) string {
	testhelper.PrepareTestRootDir(t)

	body, err := os.ReadFile(filepath.Join(testhelper.TestRoot, filename))
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
			require.NoError(t, err)
		case "/api/v4/internal/shellhorse/git_audit_event":
			w.WriteHeader(http.StatusOK)
			return
		default:
			t.Logf("Unexpected request to successAPI: %s", r.URL.EscapedPath())
			t.FailNow()
		}
	})
}

func genServerConfig(gitlabUrl, hostKeyPath string) []byte {
	return []byte(`---
user: "git"
log_file: ""
log_format: json
secret: "0123456789abcdef"
gitlab_url: "` + gitlabUrl + `"
sshd:
  listen: "127.0.0.1:0"
  proxy_protocol: true
  web_listen: ""
  host_key_files:
    - "` + hostKeyPath + `"`)
}

func buildClient(t *testing.T, addr string, hostKey ed25519.PublicKey) *ssh.Client {
	t.Helper()

	pubKey, err := ssh.NewPublicKey(hostKey)
	require.NoError(t, err)

	_, clientPrivKey, err := ed25519.GenerateKey(nil)
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
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempDir)) })

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
	extractor := regexp.MustCompile(`"tcp_address":"([0-9a-f\[\]\.:]+)"`)

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
func runSSHD(t *testing.T, apiHandler http.Handler) *ssh.Client {
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

	return buildClient(t, listenAddr, hostKey)
}

func TestDiscoverSuccess(t *testing.T) {
	handler := customHandler{
		url: "/api/v4/internal/discover",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"id": 1000, "name": "Test User", "username": "test-user"}`)
		},
	}
	client := runSSHD(t, successAPI(t, handler))

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
	client := runSSHD(t, successAPI(t, handler))

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
	client := runSSHD(t, successAPI(t, handler))
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
	client := runSSHD(t, successAPI(t, handler))
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
	handler := customHandler{
		url: "/api/v4/internal/lfs_authenticate",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"username": "test-user", "lfs_token": "testlfstoken", "repo_path": "foo", "expires_in": 7200}`)
		},
	}
	client := runSSHD(t, successAPI(t, handler))

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()

	output, err := session.Output("git-lfs-authenticate test-user/repo.git download")

	require.NoError(t, err)
	require.Equal(t, `{"header":{"Authorization":"Basic dGVzdC11c2VyOnRlc3RsZnN0b2tlbg=="},"href":"/info/lfs","expires_in":7200}
`, string(output))
}

func TestGitReceivePackSuccess(t *testing.T) {
	ensureGitalyRepository(t)

	client := runSSHD(t, successAPI(t))
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

func TestGeoGitReceivePackSuccess(t *testing.T) {
	url := startGitOverHTTPServer(t)

	handler := customHandler{
		url: "/api/v4/internal/allowed",
		caller: func(w http.ResponseWriter, _ *http.Request) {
			response := buildAllowedResponse(t, "responses/allowed_with_geo_push_payload.json")
			response = strings.Replace(response, "PRIMARY_REPO", url, 1)

			w.WriteHeader(300)
			_, err := fmt.Fprint(w, response)
			require.NoError(t, err)
		},
	}
	client := runSSHD(t, successAPI(t, handler))
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

	client := runSSHD(t, successAPI(t))
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

			output, err := io.ReadAll(stdout)
			require.NoError(t, err)

			outputLines := strings.Split(string(output), "\n")

			for i := 1; i < (len(outputLines) - 1); i++ {
				require.Regexp(t, "^[0-9a-f]{44} refs/(heads|tags)/[^ ]+", outputLines[i])
			}

			require.Equal(t, "0000", outputLines[len(outputLines)-1])
		})
	}
}

func TestGitUploadArchiveSuccess(t *testing.T) {
	ensureGitalyRepository(t)

	client := runSSHD(t, successAPI(t))
	session, stdin, stdout := newSession(t, client)
	reader := bufio.NewReader(stdout)

	err := session.Start(fmt.Sprintf("git-upload-archive %s", testRepo))
	require.NoError(t, err)

	_, err = fmt.Fprintln(stdin, "0012argument HEAD\n0000")
	require.NoError(t, err)

	line, err := reader.ReadString('\n')
	require.Equal(t, "0008ACK\n", line)
	require.NoError(t, err)

	// Gracefully close connection
	_, err = fmt.Fprintln(stdin, "0000")
	require.NoError(t, err)

	output, err := io.ReadAll(stdout)
	require.NoError(t, err)

	t.Logf("output: %q", output)
	require.Equal(t, []byte("0000"), output[len(output)-4:])
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
