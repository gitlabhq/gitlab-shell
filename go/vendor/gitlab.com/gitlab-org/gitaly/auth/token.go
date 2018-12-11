package gitalyauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	timestampThreshold = 30 * time.Second
)

var (
	errUnauthenticated = status.Errorf(codes.Unauthenticated, "authentication required")
	errDenied          = status.Errorf(codes.PermissionDenied, "permission denied")
)

// AuthInfo contains the authentication information coming from a request
type AuthInfo struct {
	Version       string
	SignedMessage []byte
	Message       string
}

// CheckToken checks the 'authentication' header of incoming gRPC
// metadata in ctx. It returns nil if and only if the token matches
// secret.
func CheckToken(ctx context.Context, secret string, targetTime time.Time) error {
	if len(secret) == 0 {
		panic("CheckToken: secret may not be empty")
	}

	authInfo, err := ExtractAuthInfo(ctx)
	if err != nil {
		return errUnauthenticated
	}

	switch authInfo.Version {
	case "v1":
		decodedToken, err := base64.StdEncoding.DecodeString(authInfo.Message)
		if err != nil {
			return errUnauthenticated
		}

		if tokensEqual(decodedToken, []byte(secret)) {
			return nil
		}
	case "v2":
		if hmacInfoValid(authInfo.Message, authInfo.SignedMessage, []byte(secret), targetTime, timestampThreshold) {
			return nil
		}
	}

	return errDenied
}

func tokensEqual(tok1, tok2 []byte) bool {
	return subtle.ConstantTimeCompare(tok1, tok2) == 1
}

// ExtractAuthInfo returns an `AuthInfo` with the data extracted from `ctx`
func ExtractAuthInfo(ctx context.Context) (*AuthInfo, error) {
	token, err := grpc_auth.AuthFromMD(ctx, "bearer")

	if err != nil {
		return nil, err
	}

	split := strings.SplitN(string(token), ".", 3)

	// v1 is base64-encoded using base64.StdEncoding, which cannot contain a ".".
	// A v1 token cannot slip through here.
	if len(split) != 3 {
		return &AuthInfo{Version: "v1", Message: token}, nil
	}

	version, sig, msg := split[0], split[1], split[2]
	decodedSig, err := hex.DecodeString(sig)
	if err != nil {
		return nil, err
	}

	return &AuthInfo{Version: version, SignedMessage: decodedSig, Message: msg}, nil
}

func hmacInfoValid(message string, signedMessage, secret []byte, targetTime time.Time, timestampThreshold time.Duration) bool {
	expectedHMAC := hmacSign(secret, message)
	if !hmac.Equal(signedMessage, expectedHMAC) {
		return false
	}

	timestamp, err := strconv.ParseInt(message, 10, 64)
	if err != nil {
		return false
	}

	issuedAt := time.Unix(timestamp, 0)
	lowerBound := targetTime.Add(-timestampThreshold)
	upperBound := targetTime.Add(timestampThreshold)

	return issuedAt.After(lowerBound) && issuedAt.Before(upperBound)
}

func hmacSign(secret []byte, message string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))

	return mac.Sum(nil)
}
