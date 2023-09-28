package lfstransfer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/git-lfs/pktline"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
)

func setupWaitGroup(t *testing.T, cmd *Command) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		_, err := cmd.Execute(context.Background())
		require.NoError(t, err)
		wg.Done()
	}()
	return wg
}

func writeCommand(t *testing.T, pl *pktline.Pktline, command string) {
	require.NoError(t, pl.WritePacketText(command))
	require.NoError(t, pl.WriteFlush())
}

func writeCommandArgs(t *testing.T, pl *pktline.Pktline, command string, args []string) {
	require.NoError(t, pl.WritePacketText(command))
	for _, arg := range args {
		require.NoError(t, pl.WritePacketText(arg))
	}
	require.NoError(t, pl.WriteFlush())
}

func writeCommandArgsAndBinaryData(t *testing.T, pl *pktline.Pktline, command string, args []string, data [][]byte) {
	require.NoError(t, pl.WritePacketText(command))
	for _, arg := range args {
		require.NoError(t, pl.WritePacketText(arg))
	}
	require.NoError(t, pl.WriteDelim())
	for _, datum := range data {
		require.NoError(t, pl.WritePacket(datum))
	}
	require.NoError(t, pl.WriteFlush())
}

func writeCommandArgsAndTextData(t *testing.T, pl *pktline.Pktline, command string, args []string, data []string) {
	require.NoError(t, pl.WritePacketText(command))
	for _, arg := range args {
		require.NoError(t, pl.WritePacketText(arg))
	}
	require.NoError(t, pl.WriteDelim())
	for _, datum := range data {
		require.NoError(t, pl.WritePacketText(datum))
	}
	require.NoError(t, pl.WriteFlush())
}

func readCapabilities(t *testing.T, pl *pktline.Pktline) {
	var caps []string
	end := false
	for !end {
		cap, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			end = true
		case 1:
			require.Fail(t, "Expected text or flush packet, got delim packet")
		default:
			caps = append(caps, cap)
		}
	}
	require.Equal(t, []string{
		"version=1",
	}, caps)
}

func readStatus(t *testing.T, pl *pktline.Pktline) string {
	// Read status.
	status, l, err := pl.ReadPacketTextWithLength()
	require.NoError(t, err)
	switch l {
	case 0:
		require.Fail(t, "Expected text, got flush packet")
	case 1:
		require.Fail(t, "Expected text, got delim packet")
	}

	// Read flush.
	_, l, err = pl.ReadPacketWithLength()
	require.NoError(t, err)
	require.Zero(t, l)

	return status
}

func readStatusArgs(t *testing.T, pl *pktline.Pktline) (status string, args []string) {
	// Read status.
	status, l, err := pl.ReadPacketTextWithLength()
	require.NoError(t, err)
	switch l {
	case 0:
		require.Fail(t, "Expected text, got flush packet")
	case 1:
		require.Fail(t, "Expected text, got delim packet")
	}

	// Read args.
	end := false
	for !end {
		arg, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			end = true
		case 1:
			require.Fail(t, "Expected text or flush packet, got delim packet")
		default:
			args = append(args, arg)
		}
	}

	return status, args
}

func readStatusArgsAndBinaryData(t *testing.T, pl *pktline.Pktline) (status string, args []string, data [][]byte) {
	// Read status.
	status, l, err := pl.ReadPacketTextWithLength()
	require.NoError(t, err)
	switch l {
	case 0:
		require.Fail(t, "Expected text, got flush packet")
	case 1:
		require.Fail(t, "Expected text, got delim packet")
	}

	// Read args.
	end := false
	for !end {
		arg, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			return status, args, nil
		case 1:
			end = true
		default:
			args = append(args, arg)
		}
	}

	// Read data.
	end = false
	for !end {
		datum, l, err := pl.ReadPacketWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			end = true
		case 1:
			require.Fail(t, "Expected data or flush packet, got delim packet")
		default:
			data = append(data, datum)
		}
	}
	return status, args, data
}

func readStatusArgsAndTextData(t *testing.T, pl *pktline.Pktline) (status string, args []string, data []string) {
	// Read status.
	status, l, err := pl.ReadPacketTextWithLength()
	require.NoError(t, err)
	switch l {
	case 0:
		require.Fail(t, "Expected text, got flush packet")
	case 1:
		require.Fail(t, "Expected text, got delim packet")
	}

	// Read args.
	end := false
	for !end {
		arg, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			return status, args, nil
		case 1:
			end = true
		default:
			args = append(args, arg)
		}
	}

	// Read data.
	end = false
	for !end {
		datum, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			end = true
		case 1:
			require.Fail(t, "Expected data or flush packet, got delim packet")
		default:
			data = append(data, datum)
		}
	}
	return status, args, data
}

func negotiateVersion(t *testing.T, pl *pktline.Pktline) {
	readCapabilities(t, pl)
	writeCommand(t, pl, "version 1")
	status, _, _ := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
}

func quit(t *testing.T, pl *pktline.Pktline) {
	writeCommand(t, pl, "quit")
	status, _, _ := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
}

func TestLfsTransferCapabilities(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferNoPermissions(t *testing.T) {
	_, cmd, _, _ := setup(t, "ro", "group/repo", "upload")
	_, err := cmd.Execute(context.Background())
	require.Equal(t, "Disallowed by API call", err.Error())
}

func TestLfsTransferBatchDownload(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndTextData(t, pl, "batch", nil, []string{
		"00000000 0",
	})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: batch is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferBatchUpload(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndTextData(t, pl, "batch", nil, []string{
		"00000000 0",
	})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: batch is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferGetObject(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "get-object 00000000")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: get-object is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferPutObject(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0"}, nil)
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: put-object is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferVerifyObject(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "verify-object 00000000", []string{"size=0"})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: verify-object is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferLock(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "lock", []string{"path=large/file"})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: lock is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferUnlock(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "unlock lock1")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: unlock is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferListLock(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroup(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "list-lock")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 405", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: list-lock is not yet supported by git-lfs-transfer. See https://gitlab.com/gitlab-org/gitlab/-/issues/336618 to track progress.",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func setup(t *testing.T, keyId string, repo string, op string) (string, *Command, *pktline.Pktline, *io.PipeReader) {
	gitalyAddress, _ := testserver.StartGitalyServer(t, "unix")
	var url string
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				var requestBody map[string]interface{}
				json.NewDecoder(r.Body).Decode(&requestBody)

				allowed := map[string]interface{}{
					"status":      true,
					"gl_id":       "1",
					"gl_key_type": "key",
					"gl_key_id":   123,
					"gl_username": "alex-doe",
					"gitaly": map[string]interface{}{
						"repository": map[string]interface{}{
							"storage_name":                     "storage_name",
							"relative_path":                    "relative_path",
							"git_object_directory":             "path/to/git_object_directory",
							"git_alternate_object_directories": []string{"path/to/git_alternate_object_directory"},
							"gl_repository":                    "group/repo",
							"gl_project_path":                  "group/project-path",
						},
						"address": gitalyAddress,
						"token":   "token",
						"features": map[string]string{
							"gitaly-feature-cache_invalidator":        "true",
							"gitaly-feature-inforef_uploadpack_cache": "false",
							"some-other-ff":                           "true",
						},
					},
				}
				disallowed := map[string]interface{}{
					"status":  false,
					"message": "Disallowed by API call",
				}

				var body map[string]interface{}
				switch {
				case requestBody["key_id"] == "rw":
					body = allowed
				case requestBody["key_id"] == "ro" && requestBody["action"] == "git-upload-pack":
					body = allowed
				default:
					body = disallowed
				}
				require.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
		{
			Path: "/api/v4/internal/lfs_authenticate",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				require.NoError(t, err)

				var request *lfsauthenticate.Request
				require.NoError(t, json.Unmarshal(b, &request))
				if request.KeyId == "rw" {
					body := map[string]interface{}{
						"username":             "john",
						"lfs_token":            "sometoken",
						"repository_http_path": fmt.Sprintf("%s/group/repo", url),
						"expires_in":           1800,
					}
					require.NoError(t, json.NewEncoder(w).Encode(body))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			},
		},
	}
	url = testserver.StartHttpServer(t, requests)

	inputSource, inputSink := io.Pipe()
	outputSource, outputSink := io.Pipe()
	errorSource, errorSink := io.Pipe()

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url, Secret: "very secret"},
		Args:       &commandargs.Shell{GitlabKeyId: keyId, SshArgs: []string{"git-lfs-transfer", repo, op}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errorSink, Out: outputSink, In: inputSource},
	}
	pl := pktline.NewPktline(outputSource, inputSink)
	return url, cmd, pl, errorSource
}
