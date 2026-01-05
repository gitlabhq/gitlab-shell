package lfstransfer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
)

const (
	largeFileContents      = "This is a large file\n"
	evenLargerFileContents = "This is an even larger file\n"
)

var (
	largeFileLen  = len(largeFileContents)
	largeFileHash = sha256.Sum256([]byte(largeFileContents))
	largeFileOid  = hex.EncodeToString(largeFileHash[:])

	evenLargerFileLen  = len(evenLargerFileContents)
	evenLargerFileHash = sha256.Sum256([]byte(evenLargerFileContents))
	evenLargerFileOid  = hex.EncodeToString(evenLargerFileHash[:])
)

func setupWaitGroupForExecute(t *testing.T, cmd *Command) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		_, err := cmd.Execute(context.Background())
		assert.NoError(t, err)
		wg.Done()
	}()

	return wg
}

func writeCommand(t *testing.T, pl *transfer.Pktline, command string) {
	require.NoError(t, pl.WritePacketText(command))
	require.NoError(t, pl.WriteFlush())
}

func writeCommandArgs(t *testing.T, pl *transfer.Pktline, command string, args []string) {
	require.NoError(t, pl.WritePacketText(command))
	for _, arg := range args {
		require.NoError(t, pl.WritePacketText(arg))
	}
	require.NoError(t, pl.WriteFlush())
}

func writeCommandArgsAndBinaryData(t *testing.T, pl *transfer.Pktline, command string, args []string, data [][]byte) {
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

func writeCommandArgsAndTextData(t *testing.T, pl *transfer.Pktline, command string, args []string, data []string) {
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

func readCapabilities(t *testing.T, pl *transfer.Pktline) {
	var caps []string
	end := false
	for !end {
		capability, l, err := pl.ReadPacketTextWithLength()
		require.NoError(t, err)
		switch l {
		case 0:
			end = true
		case 1:
			require.Fail(t, "Expected text or flush packet, got delim packet")
		default:
			caps = append(caps, capability)
		}
	}
	require.Equal(t, []string{
		"version=1",
		"locking",
	}, caps)
}

func readStatus(t *testing.T, pl *transfer.Pktline) string {
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

func readStatusArgs(t *testing.T, pl *transfer.Pktline) (status string, args []string) {
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

func readStatusArgsAndBinaryData(t *testing.T, pl *transfer.Pktline) (status string, args []string, data [][]byte) {
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

func readStatusArgsAndTextData(t *testing.T, pl *transfer.Pktline) (status string, args []string, data []string) {
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

func negotiateVersion(t *testing.T, pl *transfer.Pktline) {
	readCapabilities(t, pl)
	writeCommand(t, pl, "version 1")
	status := readStatus(t, pl)
	require.Equal(t, "status 200", status)
}

func quit(t *testing.T, pl *transfer.Pktline) {
	writeCommand(t, pl, "quit")
	status := readStatus(t, pl)
	require.Equal(t, "status 200", status)
}

func TestLfsTransferCapabilities(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
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
	url, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndTextData(t, pl, "batch", nil, []string{
		"00000000 0",
		fmt.Sprintf("%s %d", largeFileOid, largeFileLen),
		fmt.Sprintf("%s %d", evenLargerFileOid, evenLargerFileLen),
	})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, "00000000 0 noop", data[0])

	largeFileArgs := strings.Split(data[1], " ")
	require.Len(t, largeFileArgs, 5)
	require.Equal(t, largeFileOid, largeFileArgs[0])
	require.Equal(t, fmt.Sprint(largeFileLen), largeFileArgs[1])
	require.Equal(t, "download", largeFileArgs[2])

	var idArg string
	var tokenArg string
	for _, arg := range largeFileArgs[3:] {
		switch {
		case strings.HasPrefix(arg, "id="):
			idArg = arg
		case strings.HasPrefix(arg, "token="):
			tokenArg = arg
		default:
			require.Fail(t, "Unexpected batch item argument: %v", arg)
		}
	}

	idBase64, found := strings.CutPrefix(idArg, "id=")
	require.True(t, found)
	idBinary, err := base64.StdEncoding.DecodeString(idBase64)
	require.NoError(t, err)

	var id map[string]interface{}
	require.NoError(t, json.Unmarshal(idBinary, &id))
	require.Equal(t, map[string]interface{}{
		"operation": "download",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}, id)

	h := hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBase64, found := strings.CutPrefix(tokenArg, "token=")
	require.True(t, found)

	tokenBinary, err := base64.StdEncoding.DecodeString(tokenBase64)
	require.NoError(t, err)
	require.Equal(t, h.Sum(nil), tokenBinary)

	require.Equal(t, fmt.Sprintf("%s %d noop", evenLargerFileOid, evenLargerFileLen), data[2])

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferBatchUpload(t *testing.T) {
	url, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndTextData(t, pl, "batch", nil, []string{
		"00000000 0",
		fmt.Sprintf("%s %d", largeFileOid, largeFileLen),
		fmt.Sprintf("%s %d", evenLargerFileOid, evenLargerFileLen),
	})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, "00000000 0 noop", data[0])

	require.Equal(t, fmt.Sprintf("%s %d noop", largeFileOid, largeFileLen), data[1])

	evenLargerFileArgs := strings.Split(data[2], " ")
	require.Len(t, evenLargerFileArgs, 5)
	require.Equal(t, evenLargerFileOid, evenLargerFileArgs[0])
	require.Equal(t, fmt.Sprint(evenLargerFileLen), evenLargerFileArgs[1])
	require.Equal(t, "upload", evenLargerFileArgs[2])

	var idArg string
	var tokenArg string
	for _, arg := range evenLargerFileArgs[3:] {
		switch {
		case strings.HasPrefix(arg, "id="):
			idArg = arg
		case strings.HasPrefix(arg, "token="):
			tokenArg = arg
		default:
			require.Fail(t, "Unexpected batch item argument: %v", arg)
		}
	}

	idBase64, found := strings.CutPrefix(idArg, "id=")
	require.True(t, found)
	idBinary, err := base64.StdEncoding.DecodeString(idBase64)
	require.NoError(t, err)
	var id map[string]interface{}
	require.NoError(t, json.Unmarshal(idBinary, &id))
	require.Equal(t, map[string]interface{}{
		"operation": "upload",
		"oid":       evenLargerFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}, id)

	h := hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBase64, found := strings.CutPrefix(tokenArg, "token=")
	require.True(t, found)
	tokenBinary, err := base64.StdEncoding.DecodeString(tokenBase64)
	require.NoError(t, err)
	require.Equal(t, h.Sum(nil), tokenBinary)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferGetObject(t *testing.T) {
	url, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "get-object 00000000")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing id",
	}, data)

	writeCommandArgs(t, pl, "get-object 00000000", []string{"id=ggg"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 401", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing token",
	}, data)

	writeCommandArgs(t, pl, "get-object 00000000", []string{"id=ggg", "token=ggg"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid id",
	}, data)

	id := base64.StdEncoding.EncodeToString([]byte("{}"))
	writeCommandArgs(t, pl, "get-object 00000000", []string{fmt.Sprintf("id=%s", id), "token=ggg"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid token",
	}, data)

	id = base64.StdEncoding.EncodeToString([]byte("{}"))
	token := base64.StdEncoding.EncodeToString([]byte("aaa"))
	writeCommandArgs(t, pl, "get-object 00000000", []string{fmt.Sprintf("id=%s", id), fmt.Sprintf("token=%s", token)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: token hash mismatch",
	}, data)

	idJSON := map[string]interface{}{
		"operation": "download",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ := json.Marshal(idJSON)
	idBase64 := base64.StdEncoding.EncodeToString(idBinary)
	h := hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary := h.Sum(nil)
	tokenBase64 := base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgs(t, pl, fmt.Sprintf("get-object %s", largeFileOid), []string{fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)})
	status, args, binData := readStatusArgsAndBinaryData(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		fmt.Sprintf("size=%d", largeFileLen),
	}, args)
	require.Equal(t, [][]byte{[]byte(largeFileContents)}, binData)

	idJSON = map[string]interface{}{
		"operation": "download",
		"oid":       evenLargerFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, evenLargerFileOid),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgs(t, pl, fmt.Sprintf("get-object %s", evenLargerFileOid), []string{fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 404", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		fmt.Sprintf("object %s not found", evenLargerFileOid),
	}, data)

	idJSON = map[string]interface{}{
		"operation": "upload",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgs(t, pl, fmt.Sprintf("get-object %s", largeFileOid), []string{fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid operation",
	}, data)

	idJSON = map[string]interface{}{
		"operation": "download",
		"oid":       evenLargerFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgs(t, pl, fmt.Sprintf("get-object %s", largeFileOid), []string{fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid oid",
	}, data)

	idJSON = map[string]interface{}{
		"operation": "download",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/evil-url", url),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("evil secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgs(t, pl, fmt.Sprintf("get-object %s", largeFileOid), []string{fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: token hash mismatch",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferPutObject(t *testing.T) {
	url, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0"}, nil)
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing id",
	}, data)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0", "id=ggg"}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 401", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing token",
	}, data)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0", "id=ggg", "token=ggg"}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid id",
	}, data)

	id := base64.StdEncoding.EncodeToString([]byte("{}"))
	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0", fmt.Sprintf("id=%s", id), "token=ggg"}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid token",
	}, data)

	id = base64.StdEncoding.EncodeToString([]byte("{}"))
	token := base64.StdEncoding.EncodeToString([]byte("aaa"))
	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{"size=0", fmt.Sprintf("id=%s", id), fmt.Sprintf("token=%s", token)}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: token hash mismatch",
	}, data)

	idJSON := map[string]interface{}{
		"operation": "upload",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/group/noexist/gitlab-lfs/objects/%s/%d", url, largeFileOid, largeFileLen),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ := json.Marshal(idJSON)
	idBase64 := base64.StdEncoding.EncodeToString(idBinary)
	h := hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary := h.Sum(nil)
	tokenBase64 := base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgsAndBinaryData(t, pl, fmt.Sprintf("put-object %s", largeFileOid), []string{fmt.Sprintf("size=%d", largeFileLen), fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)}, [][]byte{[]byte(largeFileContents)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 404", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: not found",
	}, data)

	idJSON = map[string]interface{}{
		"operation": "upload",
		"oid":       evenLargerFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgsAndBinaryData(t, pl, fmt.Sprintf("put-object %s", evenLargerFileOid), []string{fmt.Sprintf("size=%d", evenLargerFileLen), fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)}, [][]byte{[]byte(evenLargerFileContents)})
	status = readStatus(t, pl)
	require.Equal(t, "status 200", status)

	idJSON = map[string]interface{}{
		"operation": "download",
		"oid":       evenLargerFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgsAndBinaryData(t, pl, fmt.Sprintf("put-object %s", evenLargerFileOid), []string{fmt.Sprintf("size=%d", evenLargerFileLen), fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)}, [][]byte{[]byte(evenLargerFileContents)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid operation",
	}, data)

	idJSON = map[string]interface{}{
		"operation": "upload",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("very secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgsAndBinaryData(t, pl, fmt.Sprintf("put-object %s", evenLargerFileOid), []string{fmt.Sprintf("size=%d", evenLargerFileLen), fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)}, [][]byte{[]byte(evenLargerFileContents)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid oid",
	}, data)

	idJSON = map[string]interface{}{
		"operation": "upload",
		"oid":       largeFileOid,
		"href":      fmt.Sprintf("%s/evil-url", url),
		"headers": map[string]interface{}{
			"Authorization": "Basic 1234567890",
			"Content-Type":  "application/octet-stream",
		},
	}
	idBinary, _ = json.Marshal(idJSON)
	idBase64 = base64.StdEncoding.EncodeToString(idBinary)
	h = hmac.New(sha256.New, []byte("evil secret"))
	h.Write(idBinary)
	tokenBinary = h.Sum(nil)
	tokenBase64 = base64.StdEncoding.EncodeToString(tokenBinary)
	writeCommandArgsAndBinaryData(t, pl, fmt.Sprintf("put-object %s", evenLargerFileOid), []string{fmt.Sprintf("size=%d", evenLargerFileLen), fmt.Sprintf("id=%s", idBase64), fmt.Sprintf("token=%s", tokenBase64)}, [][]byte{[]byte(evenLargerFileContents)})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: token hash mismatch",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferVerifyObject(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "verify-object 00000000", []string{"size=0"})
	status := readStatus(t, pl)
	require.Equal(t, "status 200", status)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferLock(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/1"})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 409", status)
	require.Equal(t, []string{
		"id=lock1",
		"path=/large/file/1",
		"locked-at=2023-10-03T13:56:20Z",
		"ownername=johndoe",
	}, args)
	require.Equal(t, []string{
		"conflict",
	}, data)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: forbidden",
	}, data)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/3"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 500", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"internal error",
	}, data)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/4"})
	status, args = readStatusArgs(t, pl)
	require.Equal(t, "status 201", status)
	require.Equal(t, []string{
		"id=lock4",
		"path=/large/file/4",
		"locked-at=2023-10-03T13:56:20Z",
		"ownername=johndoe",
	}, args)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/5", "refname=refs/heads/main"})
	status, args = readStatusArgs(t, pl)
	require.Equal(t, "status 201", status)
	require.Equal(t, []string{
		"id=lock5",
		"path=/large/file/5",
		"locked-at=2023-10-03T13:56:20Z",
		"ownername=johndoe",
	}, args)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferUnlock(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "unlock lock1", []string{"refname=refs/heads/main"})
	status, args := readStatusArgs(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"id=lock1",
		"path=/large/file/1",
		"locked-at=2023-10-03T13:56:20Z",
		"ownername=johndoe",
	}, args)

	writeCommandArgs(t, pl, "unlock lock2", []string{"force=true"})
	status, args = readStatusArgs(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"id=lock2",
		"path=/large/file/2",
		"locked-at=1955-11-12T22:04:00Z",
		"ownername=marty",
	}, args)

	writeCommand(t, pl, "unlock lock3")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: forbidden",
	}, data)

	writeCommand(t, pl, "unlock lock4")
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 404", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: not found",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferListLockDownload(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "download")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "list-lock")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",

		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",

		"lock lock3",
		"path lock3 /large/file/3",
		"locked-at lock3 2023-10-03T13:56:20Z",
		"ownername lock3 janedoe",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"limit=2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"next-cursor=lock3",
	}, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",

		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"cursor=lock2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",

		"lock lock3",
		"path lock3 /large/file/3",
		"locked-at lock3 2023-10-03T13:56:20Z",
		"ownername lock3 janedoe",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"id=lock1"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"path=/large/file/2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferListLockUpload(t *testing.T) {
	_, cmd, pl, _ := setup(t, "rw", "group/repo", "upload")
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "list-lock")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",
		"owner lock1 ours",

		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
		"owner lock2 theirs",

		"lock lock3",
		"path lock3 /large/file/3",
		"locked-at lock3 2023-10-03T13:56:20Z",
		"ownername lock3 janedoe",
		"owner lock3 theirs",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"limit=2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"next-cursor=lock3",
	}, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",
		"owner lock1 ours",

		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
		"owner lock2 theirs",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"cursor=lock2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
		"owner lock2 theirs",

		"lock lock3",
		"path lock3 /large/file/3",
		"locked-at lock3 2023-10-03T13:56:20Z",
		"ownername lock3 janedoe",
		"owner lock3 theirs",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"id=lock1"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock1",
		"path lock1 /large/file/1",
		"locked-at lock1 2023-10-03T13:56:20Z",
		"ownername lock1 johndoe",
		"owner lock1 ours",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"path=/large/file/2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"lock lock2",
		"path lock2 /large/file/2",
		"locked-at lock2 1955-11-12T22:04:00Z",
		"ownername lock2 marty",
		"owner lock2 theirs",
	}, data)

	quit(t, pl)
	wg.Wait()
}

type Owner struct {
	Name string `json:"name"`
}
type LockInfo struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	LockedAt string `json:"locked_at"`
	*Owner   `json:"owner"`
}

func listLocks(cursor string, limit int, refspec string, id string, path string) (locks []*LockInfo, nextCursor string) {
	allLocks := []struct {
		Refspec string
		*LockInfo
	}{
		{
			Refspec: "main",
			LockInfo: &LockInfo{
				ID:       "lock1",
				Path:     "/large/file/1",
				LockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				Owner: &Owner{
					Name: "johndoe",
				},
			},
		},
		{
			Refspec: "my-branch",
			LockInfo: &LockInfo{
				ID:       "lock2",
				Path:     "/large/file/2",
				LockedAt: time.Date(1955, 11, 12, 22, 04, 0, 0, time.UTC).Format(time.RFC3339),
				Owner: &Owner{
					Name: "marty",
				},
			},
		},
		{
			Refspec: "",
			LockInfo: &LockInfo{
				ID:       "lock3",
				Path:     "/large/file/3",
				LockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				Owner: &Owner{
					Name: "janedoe",
				},
			},
		},
	}

	for _, lock := range allLocks {
		if cursor != "" && cursor != lock.ID {
			continue
		}
		cursor = ""
		if len(locks) >= limit {
			nextCursor = lock.ID
			break
		}

		if refspec != "" && refspec != lock.Refspec {
			continue
		}
		if id != "" && id != lock.ID {
			continue
		}
		if path != "" && path != lock.Path {
			continue
		}
		locks = append(locks, lock.LockInfo)
	}
	return locks, nextCursor
}

func setup(t *testing.T, keyID string, repo string, op string) (string, *Command, *transfer.Pktline, *io.PipeReader) {
	var url string

	gitalyAddress, _ := testserver.StartGitalyServer(t, "unix")
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
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
		{
			Path: "/api/v4/internal/lfs_authenticate",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				assert.NoError(t, err)

				var request *lfsauthenticate.Request
				assert.NoError(t, json.Unmarshal(b, &request))
				if request.KeyID == "rw" {
					body := map[string]interface{}{
						"username":             "john",
						"lfs_token":            "sometoken",
						"repository_http_path": fmt.Sprintf("%s/group/repo", url),
						"expires_in":           1800,
					}
					assert.NoError(t, json.NewEncoder(w).Encode(body))
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
			},
		},
		{
			Path: "/group/repo/info/lfs/objects/batch",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte("john:sometoken"))), r.Header.Get("Authorization"))

				var requestBody map[string]interface{}
				json.NewDecoder(r.Body).Decode(&requestBody)

				reqObjects := requestBody["objects"].([]interface{})
				retObjects := make([]map[string]interface{}, 0)
				for _, o := range reqObjects {
					reqObject := o.(map[string]interface{})
					retObject := map[string]interface{}{
						"oid": reqObject["oid"],
					}
					switch reqObject["oid"] {
					case largeFileOid:
						retObject["size"] = largeFileLen
						if op == "download" {
							retObject["actions"] = map[string]interface{}{
								"download": map[string]interface{}{
									"href": fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
									"header": map[string]interface{}{
										"Authorization": "Basic 1234567890",
										"Content-Type":  "application/octet-stream",
									},
								},
							}
						}
					case evenLargerFileOid:
						assert.Equal(t, evenLargerFileLen, int(reqObject["size"].(float64)))
						retObject["size"] = evenLargerFileLen
						if op == "upload" {
							retObject["actions"] = map[string]interface{}{
								"upload": map[string]interface{}{
									"href": fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
									"header": map[string]interface{}{
										"Authorization": "Basic 1234567890",
										"Content-Type":  "application/octet-stream",
									},
								},
							}
						}
					default:
						retObject["size"] = reqObject["size"]
						retObject["error"] = map[string]interface{}{
							"code":    404,
							"message": "Not found",
						}
					}
					retObjects = append(retObjects, retObject)
				}

				retBody := map[string]interface{}{
					"objects": retObjects,
				}
				body, _ := json.Marshal(retBody)
				w.Write(body)
			},
		},
		{
			Path: "/evil-url",
			Handler: func(_ http.ResponseWriter, _ *http.Request) {
				assert.Fail(t, "An attacker accessed an evil URL")
			},
		},
		{
			Path: fmt.Sprintf("/group/repo/gitlab-lfs/objects/%s", largeFileOid),
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Basic 1234567890", r.Header.Get("Authorization"))
				w.Write([]byte(largeFileContents))
			},
		},
		{
			Path: fmt.Sprintf("/group/repo/gitlab-lfs/objects/%s/%d", evenLargerFileOid, evenLargerFileLen),
			Handler: func(_ http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "Basic 1234567890", r.Header.Get("Authorization"))
				body, _ := io.ReadAll(r.Body)
				assert.Equal(t, []byte(evenLargerFileContents), body)
			},
		},
		{
			Path: "/group/repo/info/lfs/locks/verify",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				requestJSON := &struct {
					Cursor string `json:"cursor"`
					Limit  int    `json:"limit"`
					Ref    struct {
						Name string `json:"name"`
					} `json:"ref"`
				}{}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(requestJSON))

				bodyJSON := &struct {
					Ours       []*LockInfo `json:"ours,omitempty"`
					Theirs     []*LockInfo `json:"theirs,omitempty"`
					NextCursor string      `json:"next_cursor,omitempty"`
				}{}
				var locks []*LockInfo
				locks, bodyJSON.NextCursor = listLocks(requestJSON.Cursor, requestJSON.Limit, requestJSON.Ref.Name, r.URL.Query().Get("id"), r.URL.Query().Get("path"))
				for _, lock := range locks {
					if lock.ID == "lock1" {
						bodyJSON.Ours = append(bodyJSON.Ours, lock)
					} else {
						bodyJSON.Theirs = append(bodyJSON.Theirs, lock)
					}
				}

				assert.NoError(t, json.NewEncoder(w).Encode(bodyJSON))
			},
		},
		{
			Path: "/group/repo/info/lfs/locks",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					bodyJSON := &struct {
						Locks      []*LockInfo `json:"locks,omitempty"`
						NextCursor string      `json:"next_cursor,omitempty"`
					}{}
					limit := 100
					if r.URL.Query().Has("limit") {
						l, err := strconv.Atoi(r.URL.Query().Get("limit"))
						assert.NoError(t, err)
						limit = l
					}
					bodyJSON.Locks, bodyJSON.NextCursor = listLocks(r.URL.Query().Get("cursor"), limit, r.URL.Query().Get("refspec"), r.URL.Query().Get("id"), r.URL.Query().Get("path"))
					assert.NoError(t, json.NewEncoder(w).Encode(bodyJSON))
				case http.MethodPost:
					var body map[string]interface{}
					reader := json.NewDecoder(r.Body)
					reader.Decode(&body)

					var response map[string]interface{}
					switch body["path"] {
					case "/large/file/1":
						response = map[string]interface{}{
							"lock": map[string]interface{}{
								"id":        "lock1",
								"path":      "/large/file/1",
								"locked_at": time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
								"owner": map[string]interface{}{
									"name": "johndoe",
								},
							},
							"message": "already created lock",
						}
						w.WriteHeader(http.StatusConflict)
					case "/large/file/2":
						response = map[string]interface{}{
							"message": "no permission",
						}
						w.WriteHeader(http.StatusForbidden)
					case "/large/file/4":
						response = map[string]interface{}{
							"lock": map[string]interface{}{
								"id":        "lock4",
								"path":      "/large/file/4",
								"locked_at": time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
								"owner": map[string]interface{}{
									"name": "johndoe",
								},
							},
						}
						w.WriteHeader(http.StatusCreated)
					case "/large/file/5":
						ref := body["ref"].(map[string]interface{})
						assert.Equal(t, "refs/heads/main", ref["name"])
						response = map[string]interface{}{
							"lock": map[string]interface{}{
								"id":        "lock5",
								"path":      "/large/file/5",
								"locked_at": time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
								"owner": map[string]interface{}{
									"name": "johndoe",
								},
							},
						}
						w.WriteHeader(http.StatusCreated)
					default:
						response = map[string]interface{}{
							"message": "internal error",
						}
						w.WriteHeader(http.StatusInternalServerError)
					}
					writer := json.NewEncoder(w)
					writer.Encode(response)
				}
			},
		},
		{
			Path: "/group/repo/info/lfs/locks/lock1/unlock",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				var body map[string]interface{}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, map[string]interface{}{
					"ref": map[string]interface{}{
						"name": "refs/heads/main",
					},
					"force": false,
				}, body)

				lock := map[string]interface{}{
					"lock": map[string]interface{}{
						"id":        "lock1",
						"path":      "/large/file/1",
						"locked_at": time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
						"owner": map[string]interface{}{
							"name": "johndoe",
						},
					},
				}
				writer := json.NewEncoder(w)
				writer.Encode(lock)
			},
		},
		{
			Path: "/group/repo/info/lfs/locks/lock2/unlock",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				var body map[string]interface{}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, map[string]interface{}{
					"force": true,
				}, body)

				lock := map[string]interface{}{
					"lock": map[string]interface{}{
						"id":        "lock2",
						"path":      "/large/file/2",
						"locked_at": time.Date(1955, 11, 12, 22, 4, 0, 0, time.UTC).Format(time.RFC3339),
						"owner": map[string]interface{}{
							"name": "marty",
						},
					},
				}
				writer := json.NewEncoder(w)
				writer.Encode(lock)
			},
		},
		{
			Path: "/group/repo/info/lfs/locks/lock3/unlock",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				var body map[string]interface{}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, map[string]interface{}{
					"force": false,
				}, body)

				lock := map[string]interface{}{
					"message": "forbidden",
				}
				w.WriteHeader(http.StatusForbidden)
				writer := json.NewEncoder(w)
				writer.Encode(lock)
			},
		},
		{
			Path: "/group/repo/info/lfs/locks/lock4/unlock",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				var body map[string]interface{}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, map[string]interface{}{
					"force": false,
				}, body)

				lock := map[string]interface{}{
					"message": "not found",
				}
				w.WriteHeader(http.StatusNotFound)
				writer := json.NewEncoder(w)
				writer.Encode(lock)
			},
		},
	}

	url = testserver.StartHTTPServer(t, requests)

	inputSource, inputSink := io.Pipe()
	outputSource, outputSink := io.Pipe()
	errorSource, errorSink := io.Pipe()

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url, Secret: "very secret"},
		Args:       &commandargs.Shell{GitlabKeyID: keyID, SSHArgs: []string{"git-lfs-transfer", repo, op}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errorSink, Out: outputSink, In: inputSource},
	}
	pl := transfer.NewPktline(outputSource, inputSink)

	return url, cmd, pl, errorSource
}
