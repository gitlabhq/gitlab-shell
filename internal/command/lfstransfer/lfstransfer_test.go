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

	"github.com/git-lfs/pktline"
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

	// JSON field names used in LFS batch responses
	fieldOid           = "oid"
	fieldHref          = "href"
	fieldHeaders       = "headers"
	fieldAuthorization = "Authorization"
	fieldContentType   = "Content-Type"
	fieldMessage       = "message"
	fieldLock          = "lock"
	fieldPath          = "path"
	fieldLockedAt      = "locked_at"
	fieldOwner         = "owner"
	fieldName          = "name"

	// Test argument values
	testIDGgg       = "id=ggg"
	testTokenGgg    = "token=ggg"
	testSizeZero    = "size=0"
	testOperation   = "operation"
	testAuthHeader  = "Basic 1234567890"
	testContentType = "application/octet-stream"

	// Error messages
	errTokenHashMismatch = "error: token hash mismatch"

	// Lock/file paths and IDs
	lockID1    = "lock1"
	lockID2    = "lock2"
	lockID3    = "lock3"
	filePath1  = "/large/file/1"
	filePath2  = "/large/file/2"
	filePath3  = "/large/file/3"
	lockedAt1  = "2023-10-03T13:56:20Z"
	lockedAt2  = "1955-11-12T22:04:00Z"
	ownerJohn  = "johndoe"
	ownerMarty = "marty"
	ownerJane  = "janedoe"

	// Lock list output entries
	lockLock1        = "lock lock1"
	pathLock1        = "path lock1 /large/file/1"
	lockedAtLock1    = "locked-at lock1 2023-10-03T13:56:20Z"
	ownernameLock1   = "ownername lock1 johndoe"
	lockLock2        = "lock lock2"
	pathLock2        = "path lock2 /large/file/2"
	lockedAtLock2    = "locked-at lock2 1955-11-12T22:04:00Z"
	ownernameLock2   = "ownername lock2 marty"
	lockLock3        = "lock lock3"
	pathLock3        = "path lock3 /large/file/3"
	lockedAtLock3    = "locked-at lock3 2023-10-03T13:56:20Z"
	ownernameLock3   = "ownername lock3 janedoe"
	ownerLock1Ours   = "owner lock1 ours"
	ownerLock2Theirs = "owner lock2 theirs"

	// Lock command args (pktline format)
	argIDLock1    = "id=lock1"
	argPathFile1  = "path=/large/file/1"
	argPathFile2  = "path=/large/file/2"
	argLockedAt1  = "locked-at=2023-10-03T13:56:20Z"
	argOwnername1 = "ownername=johndoe"
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
	_, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferNoPermissions(t *testing.T) {
	_, cmd, _ := setup(t, "ro", opUpload)
	_, err := cmd.Execute(context.Background())
	require.Equal(t, "Disallowed by API call", err.Error())
}

func TestLfsTransferBatchDownload(t *testing.T) {
	url, cmd, pl := setup(t, "rw", opDownload)
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
			require.Failf(t, "Unexpected batch item argument", "%v", arg)
		}
	}

	idBase64, found := strings.CutPrefix(idArg, "id=")
	require.True(t, found)
	idBinary, err := base64.StdEncoding.DecodeString(idBase64)
	require.NoError(t, err)

	var id map[string]interface{}
	require.NoError(t, json.Unmarshal(idBinary, &id))
	require.Equal(t, map[string]interface{}{
		testOperation: opDownload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
	url, cmd, pl := setup(t, "rw", opUpload)
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
	require.Equal(t, opUpload, evenLargerFileArgs[2])

	var idArg string
	var tokenArg string
	for _, arg := range evenLargerFileArgs[3:] {
		switch {
		case strings.HasPrefix(arg, "id="):
			idArg = arg
		case strings.HasPrefix(arg, "token="):
			tokenArg = arg
		default:
			require.Failf(t, "Unexpected batch item argument", "%v", arg)
		}
	}

	idBase64, found := strings.CutPrefix(idArg, "id=")
	require.True(t, found)
	idBinary, err := base64.StdEncoding.DecodeString(idBase64)
	require.NoError(t, err)
	var id map[string]interface{}
	require.NoError(t, json.Unmarshal(idBinary, &id))
	require.Equal(t, map[string]interface{}{
		testOperation: opUpload,
		fieldOid:      evenLargerFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
	url, cmd, pl := setup(t, "rw", opDownload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "get-object 00000000")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing id",
	}, data)

	writeCommandArgs(t, pl, "get-object 00000000", []string{testIDGgg})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 401", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing token",
	}, data)

	writeCommandArgs(t, pl, "get-object 00000000", []string{testIDGgg, testTokenGgg})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid id",
	}, data)

	id := base64.StdEncoding.EncodeToString([]byte("{}"))
	writeCommandArgs(t, pl, "get-object 00000000", []string{fmt.Sprintf("id=%s", id), testTokenGgg})
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
		errTokenHashMismatch,
	}, data)

	idJSON := map[string]interface{}{
		testOperation: opDownload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opDownload,
		fieldOid:      evenLargerFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, evenLargerFileOid),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opUpload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opDownload,
		fieldOid:      evenLargerFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", url, largeFileOid),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opDownload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/evil-url", url),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		errTokenHashMismatch,
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferPutObject(t *testing.T) {
	url, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{testSizeZero}, nil)
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing id",
	}, data)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{testSizeZero, testIDGgg}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 401", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: missing token",
	}, data)

	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{testSizeZero, testIDGgg, testTokenGgg}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid id",
	}, data)

	id := base64.StdEncoding.EncodeToString([]byte("{}"))
	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{testSizeZero, fmt.Sprintf("id=%s", id), testTokenGgg}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 400", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		"error: invalid token",
	}, data)

	id = base64.StdEncoding.EncodeToString([]byte("{}"))
	token := base64.StdEncoding.EncodeToString([]byte("aaa"))
	writeCommandArgsAndBinaryData(t, pl, "put-object 00000000", []string{testSizeZero, fmt.Sprintf("id=%s", id), fmt.Sprintf("token=%s", token)}, nil)
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 403", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		errTokenHashMismatch,
	}, data)

	idJSON := map[string]interface{}{
		testOperation: opUpload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/group/noexist/gitlab-lfs/objects/%s/%d", url, largeFileOid, largeFileLen),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opUpload,
		fieldOid:      evenLargerFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opDownload,
		fieldOid:      evenLargerFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opUpload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", url, evenLargerFileOid, evenLargerFileLen),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		testOperation: opUpload,
		fieldOid:      largeFileOid,
		fieldHref:     fmt.Sprintf("%s/evil-url", url),
		fieldHeaders: map[string]interface{}{
			fieldAuthorization: testAuthHeader,
			fieldContentType:   testContentType,
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
		errTokenHashMismatch,
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferVerifyObject(t *testing.T) {
	_, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "verify-object 00000000", []string{testSizeZero})
	status := readStatus(t, pl)
	require.Equal(t, "status 200", status)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferLock(t *testing.T) {
	_, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "lock", []string{argPathFile1})
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 409", status)
	require.Equal(t, []string{
		argIDLock1,
		argPathFile1,
		argLockedAt1,
		argOwnername1,
	}, args)
	require.Equal(t, []string{
		"conflict",
	}, data)

	writeCommandArgs(t, pl, "lock", []string{argPathFile2})
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
		argLockedAt1,
		argOwnername1,
	}, args)

	writeCommandArgs(t, pl, "lock", []string{"path=/large/file/5", "refname=refs/heads/main"})
	status, args = readStatusArgs(t, pl)
	require.Equal(t, "status 201", status)
	require.Equal(t, []string{
		"id=lock5",
		"path=/large/file/5",
		argLockedAt1,
		argOwnername1,
	}, args)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferUnlock(t *testing.T) {
	_, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommandArgs(t, pl, "unlock lock1", []string{"refname=refs/heads/main"})
	status, args := readStatusArgs(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		argIDLock1,
		argPathFile1,
		argLockedAt1,
		argOwnername1,
	}, args)

	writeCommandArgs(t, pl, "unlock lock2", []string{"force=true"})
	status, args = readStatusArgs(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"id=lock2",
		argPathFile2,
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
	_, cmd, pl := setup(t, "rw", opDownload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "list-lock")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,

		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,

		lockLock3,
		pathLock3,
		lockedAtLock3,
		ownernameLock3,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"limit=2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"next-cursor=lock3",
	}, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,

		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"cursor=lock2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,

		lockLock3,
		pathLock3,
		lockedAtLock3,
		ownernameLock3,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{argIDLock1})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{argPathFile2})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
	}, data)

	quit(t, pl)
	wg.Wait()
}

func TestLfsTransferListLockUpload(t *testing.T) {
	_, cmd, pl := setup(t, "rw", opUpload)
	wg := setupWaitGroupForExecute(t, cmd)
	negotiateVersion(t, pl)

	writeCommand(t, pl, "list-lock")
	status, args, data := readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,
		ownerLock1Ours,

		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
		ownerLock2Theirs,

		lockLock3,
		pathLock3,
		lockedAtLock3,
		ownernameLock3,
		"owner lock3 theirs",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"limit=2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Equal(t, []string{
		"next-cursor=lock3",
	}, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,
		ownerLock1Ours,

		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
		ownerLock2Theirs,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{"cursor=lock2"})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
		ownerLock2Theirs,

		lockLock3,
		pathLock3,
		lockedAtLock3,
		ownernameLock3,
		"owner lock3 theirs",
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{argIDLock1})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock1,
		pathLock1,
		lockedAtLock1,
		ownernameLock1,
		ownerLock1Ours,
	}, data)

	writeCommandArgs(t, pl, "list-lock", []string{argPathFile2})
	status, args, data = readStatusArgsAndTextData(t, pl)
	require.Equal(t, "status 200", status)
	require.Empty(t, args)
	require.Equal(t, []string{
		lockLock2,
		pathLock2,
		lockedAtLock2,
		ownernameLock2,
		ownerLock2Theirs,
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
				ID:       lockID1,
				Path:     filePath1,
				LockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				Owner: &Owner{
					Name: ownerJohn,
				},
			},
		},
		{
			Refspec: "my-branch",
			LockInfo: &LockInfo{
				ID:       lockID2,
				Path:     filePath2,
				LockedAt: time.Date(1955, 11, 12, 22, 04, 0, 0, time.UTC).Format(time.RFC3339),
				Owner: &Owner{
					Name: "marty",
				},
			},
		},
		{
			Refspec: "",
			LockInfo: &LockInfo{
				ID:       lockID3,
				Path:     filePath3,
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

func setup(t *testing.T, keyID string, op string) (string, *Command, *pktline.Pktline) {
	var url string
	repo := "group/repo"

	gitalyAddress, _ := testserver.StartGitalyServer(t, "unix")
	requests := buildTestRequestHandlers(t, &url, gitalyAddress, op)

	url = testserver.StartHTTPServer(t, requests)

	inputSource, inputSink := io.Pipe()
	outputSource, outputSink := io.Pipe()
	_, errorSink := io.Pipe()

	cmd := &Command{
		Config:     &config.Config{GitlabURL: url, Secret: "very secret"},
		Args:       &commandargs.Shell{GitlabKeyID: keyID, SSHArgs: []string{"git-lfs-transfer", repo, op}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errorSink, Out: outputSink, In: inputSource},
	}
	pl := pktline.NewPktline(outputSource, inputSink)

	return url, cmd, pl
}

func buildTestRequestHandlers(t *testing.T, url *string, gitalyAddress string, op string) []testserver.TestRequestHandler {
	return []testserver.TestRequestHandler{
		buildAllowedHandler(t, gitalyAddress),
		buildLFSAuthenticateHandler(t, url),
		buildBatchHandler(t, url, op),
		buildEvilURLHandler(t),
		buildLargeFileObjectHandler(t),
		buildEvenLargerFileObjectHandler(t),
		buildLocksVerifyHandler(t),
		buildLocksHandler(t),
		buildUnlockLock1Handler(t),
		buildUnlockLock2Handler(t),
		buildUnlockLock3Handler(t),
		buildUnlockLock4Handler(t),
	}
}

func buildAllowedHandler(t *testing.T, gitalyAddress string) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
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
				"status":     false,
				fieldMessage: "Disallowed by API call",
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
	}
}

func buildLFSAuthenticateHandler(t *testing.T, url *string) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
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
					"repository_http_path": fmt.Sprintf("%s/group/repo", *url),
					"expires_in":           1800,
				}
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
		},
	}
}

func buildBatchHandler(t *testing.T, url *string, op string) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
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
					fieldOid: reqObject["oid"],
				}
				switch reqObject["oid"] {
				case largeFileOid:
					retObject["size"] = largeFileLen
					if op == "download" {
						retObject["actions"] = map[string]interface{}{
							"download": map[string]interface{}{
								fieldHref: fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s", *url, largeFileOid),
								"header": map[string]interface{}{
									fieldAuthorization: testAuthHeader,
									fieldContentType:   testContentType,
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
								fieldHref: fmt.Sprintf("%s/group/repo/gitlab-lfs/objects/%s/%d", *url, evenLargerFileOid, evenLargerFileLen),
								"header": map[string]interface{}{
									fieldAuthorization: testAuthHeader,
									fieldContentType:   testContentType,
								},
							},
						}
					}
				default:
					retObject["size"] = reqObject["size"]
					retObject["error"] = map[string]interface{}{
						"code":       404,
						fieldMessage: "Not found",
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
	}
}

func buildEvilURLHandler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/evil-url",
		Handler: func(_ http.ResponseWriter, _ *http.Request) {
			assert.Fail(t, "An attacker accessed an evil URL")
		},
	}
}

func buildLargeFileObjectHandler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: fmt.Sprintf("/group/repo/gitlab-lfs/objects/%s", largeFileOid),
		Handler: func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, testAuthHeader, r.Header.Get("Authorization"))
			w.Write([]byte(largeFileContents))
		},
	}
}

func buildEvenLargerFileObjectHandler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: fmt.Sprintf("/group/repo/gitlab-lfs/objects/%s/%d", evenLargerFileOid, evenLargerFileLen),
		Handler: func(_ http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, testAuthHeader, r.Header.Get("Authorization"))
			body, _ := io.ReadAll(r.Body)
			assert.Equal(t, []byte(evenLargerFileContents), body)
		},
	}
}

func buildLocksVerifyHandler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
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
				if lock.ID == lockID1 {
					bodyJSON.Ours = append(bodyJSON.Ours, lock)
				} else {
					bodyJSON.Theirs = append(bodyJSON.Theirs, lock)
				}
			}

			assert.NoError(t, json.NewEncoder(w).Encode(bodyJSON))
		},
	}
}

func buildLocksHandler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/group/repo/info/lfs/locks",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handleLocksGet(t, w, r)
			case http.MethodPost:
				handleLocksPost(t, w, r)
			}
		},
	}
}

func handleLocksGet(t *testing.T, w http.ResponseWriter, r *http.Request) {
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
}

func handleLocksPost(t *testing.T, w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	reader := json.NewDecoder(r.Body)
	reader.Decode(&body)

	var response map[string]interface{}
	switch body["path"] {
	case filePath1:
		response = map[string]interface{}{
			fieldLock: map[string]interface{}{
				"id":          lockID1,
				fieldPath:     filePath1,
				fieldLockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				fieldOwner: map[string]interface{}{
					fieldName: ownerJohn,
				},
			},
			fieldMessage: "already created lock",
		}
		w.WriteHeader(http.StatusConflict)
	case filePath2:
		response = map[string]interface{}{
			fieldMessage: "no permission",
		}
		w.WriteHeader(http.StatusForbidden)
	case "/large/file/4":
		response = map[string]interface{}{
			fieldLock: map[string]interface{}{
				"id":          "lock4",
				fieldPath:     "/large/file/4",
				fieldLockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				fieldOwner: map[string]interface{}{
					fieldName: ownerJohn,
				},
			},
		}
		w.WriteHeader(http.StatusCreated)
	case "/large/file/5":
		ref := body["ref"].(map[string]interface{})
		assert.Equal(t, "refs/heads/main", ref["name"])
		response = map[string]interface{}{
			fieldLock: map[string]interface{}{
				"id":          "lock5",
				fieldPath:     "/large/file/5",
				fieldLockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
				fieldOwner: map[string]interface{}{
					fieldName: ownerJohn,
				},
			},
		}
		w.WriteHeader(http.StatusCreated)
	default:
		response = map[string]interface{}{
			fieldMessage: "internal error",
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
	writer := json.NewEncoder(w)
	writer.Encode(response)
}

func buildUnlockLock1Handler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/group/repo/info/lfs/locks/lock1/unlock",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]interface{}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, map[string]interface{}{
				"ref": map[string]interface{}{
					fieldName: "refs/heads/main",
				},
				argForce: false,
			}, body)

			lock := map[string]interface{}{
				fieldLock: map[string]interface{}{
					"id":          lockID1,
					fieldPath:     filePath1,
					fieldLockedAt: time.Date(2023, 10, 3, 13, 56, 20, 0, time.UTC).Format(time.RFC3339),
					fieldOwner: map[string]interface{}{
						fieldName: ownerJohn,
					},
				},
			}
			writer := json.NewEncoder(w)
			writer.Encode(lock)
		},
	}
}

func buildUnlockLock2Handler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/group/repo/info/lfs/locks/lock2/unlock",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]interface{}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, map[string]interface{}{
				argForce: true,
			}, body)

			lock := map[string]interface{}{
				fieldLock: map[string]interface{}{
					"id":          lockID2,
					fieldPath:     filePath2,
					fieldLockedAt: time.Date(1955, 11, 12, 22, 4, 0, 0, time.UTC).Format(time.RFC3339),
					fieldOwner: map[string]interface{}{
						fieldName: "marty",
					},
				},
			}
			writer := json.NewEncoder(w)
			writer.Encode(lock)
		},
	}
}

func buildUnlockLock3Handler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/group/repo/info/lfs/locks/lock3/unlock",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]interface{}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, map[string]interface{}{
				argForce: false,
			}, body)

			lock := map[string]interface{}{
				fieldMessage: "forbidden",
			}
			w.WriteHeader(http.StatusForbidden)
			writer := json.NewEncoder(w)
			writer.Encode(lock)
		},
	}
}

func buildUnlockLock4Handler(t *testing.T) testserver.TestRequestHandler {
	return testserver.TestRequestHandler{
		Path: "/group/repo/info/lfs/locks/lock4/unlock",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]interface{}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, map[string]interface{}{
				argForce: false,
			}, body)

			lock := map[string]interface{}{
				fieldMessage: "not found",
			}
			w.WriteHeader(http.StatusNotFound)
			writer := json.NewEncoder(w)
			writer.Encode(lock)
		},
	}
}
