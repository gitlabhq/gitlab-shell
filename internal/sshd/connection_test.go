package sshd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

type rejectCall struct {
	reason  ssh.RejectionReason
	message string
}

type fakeNewChannel struct {
	channelType string
	extraData   []byte
	acceptErr   error

	acceptCh chan struct{}
	rejectCh chan rejectCall
}

func (f *fakeNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	if f.acceptCh != nil {
		f.acceptCh <- struct{}{}
	}

	return nil, nil, f.acceptErr
}

func (f *fakeNewChannel) Reject(reason ssh.RejectionReason, message string) error {
	if f.rejectCh != nil {
		f.rejectCh <- rejectCall{reason: reason, message: message}
	}

	return nil
}

func (f *fakeNewChannel) ChannelType() string {
	return f.channelType
}

func (f *fakeNewChannel) ExtraData() []byte {
	return f.extraData
}

type fakeConn struct {
	ssh.Conn

	sentRequestName string
	mu              sync.Mutex
}

func (f *fakeConn) SentRequestName() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.sentRequestName
}

func (f *fakeConn) SendRequest(name string, _ bool, _ []byte) (bool, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sentRequestName = name

	return true, []byte("I am a response"), nil
}

func setup(newChannel *fakeNewChannel) (*connection, chan ssh.NewChannel) {
	var sessionsNum int64 = 1
	cfg := &config.Config{Server: config.ServerConfig{ConcurrentSessionsLimit: sessionsNum}}
	conn := &connection{cfg: cfg, concurrentSessions: semaphore.NewWeighted(sessionsNum)}

	chans := make(chan ssh.NewChannel, 1)
	chans <- newChannel

	return conn, chans
}

func TestPanicDuringSessionIsRecovered(t *testing.T) {
	newChannel := &fakeNewChannel{channelType: "session"}
	conn, chans := setup(newChannel)

	numSessions := 0
	require.NotPanics(t, func() {
		conn.handleRequests(context.Background(), nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
			numSessions++
			close(chans)
			panic("This is a panic")
		})
	})

	require.Equal(t, 1, numSessions)
}

func TestUnknownChannelType(t *testing.T) {
	rejectCh := make(chan rejectCall)
	defer close(rejectCh)

	newChannel := &fakeNewChannel{channelType: "unknown session", rejectCh: rejectCh}
	conn, chans := setup(newChannel)

	go func() {
		conn.handleRequests(context.Background(), nil, chans, nil)
	}()

	rejectionData := <-rejectCh

	expectedRejection := rejectCall{reason: ssh.UnknownChannelType, message: "unknown channel type"}
	require.Equal(t, expectedRejection, rejectionData)
}

func TestTooManySessions(t *testing.T) {
	rejectCh := make(chan rejectCall)
	defer close(rejectCh)

	newChannel := &fakeNewChannel{channelType: "session", rejectCh: rejectCh}
	conn, chans := setup(newChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		conn.handleRequests(context.Background(), nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
			<-ctx.Done() // Keep the accepted channel open until the end of the test
			return nil
		})
	}()

	chans <- newChannel
	require.Equal(t, rejectCall{reason: ssh.ResourceShortage, message: "too many concurrent sessions"}, <-rejectCh)
}

func TestAcceptSessionSucceeds(t *testing.T) {
	newChannel := &fakeNewChannel{channelType: "session"}
	conn, chans := setup(newChannel)
	ctx := context.Background()

	channelHandled := false
	conn.handleRequests(ctx, nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
		channelHandled = true
		close(chans)
		return nil
	})

	require.True(t, channelHandled)
}

func TestAcceptSessionFails(t *testing.T) {
	acceptCh := make(chan struct{})
	defer close(acceptCh)

	acceptErr := errors.New("some failure")
	newChannel := &fakeNewChannel{channelType: "session", acceptCh: acceptCh, acceptErr: acceptErr}
	conn, chans := setup(newChannel)
	ctx := context.Background()

	channelHandled := false
	go func() {
		conn.handleRequests(ctx, nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
			channelHandled = true
			return nil
		})
	}()

	require.Equal(t, struct{}{}, <-acceptCh)

	// Waits until the number of sessions is back to 0, since we can only have 1
	conn.concurrentSessions.Acquire(context.Background(), 1)
	defer conn.concurrentSessions.Release(1)

	require.False(t, channelHandled)
}

func TestClientAliveInterval(t *testing.T) {
	f := &fakeConn{}

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	conn := &connection{}
	go conn.sendKeepAliveMsg(context.Background(), &ssh.ServerConn{Conn: f, Permissions: nil}, ticker)

	require.Eventually(t, func() bool { return KeepAliveMsg == f.SentRequestName() }, time.Second, time.Millisecond)
}

func TestSessionsMetrics(t *testing.T) {
	// Unfortunately, there is no working way to reset Counter (not CounterVec)
	// https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#pkg-index
	initialSessionsTotal := testutil.ToFloat64(metrics.SliSshdSessionsTotal)
	initialSessionsErrorTotal := testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal)

	newChannel := &fakeNewChannel{channelType: "session"}
	conn, chans := setup(newChannel)
	ctx := context.Background()

	conn.handleRequests(ctx, nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
		close(chans)
		return errors.New("custom error")
	})

	eventuallyInDelta(t, initialSessionsTotal+1, testutil.ToFloat64(metrics.SliSshdSessionsTotal))
	eventuallyInDelta(t, initialSessionsErrorTotal+1, testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal))

	for i, ignoredError := range []struct {
		desc string
		err  error
	}{
		{"canceled requests", grpcstatus.Error(grpccodes.Canceled, "canceled")},
		{"unavailable Gitaly", grpcstatus.Error(grpccodes.Unavailable, "unavailable")},
		{"api error", &client.APIError{Msg: "api error"}},
		{"disallowed command", disallowedcommand.Error},
		{"not our ref", grpcstatus.Error(grpccodes.Internal, `rpc error: code = Internal desc = cmd wait: exit status 128, stderr: "fatal: git upload-pack: not our ref 9106d18f6a1b8022f6517f479696f3e3ea5e68c1"`)},
	} {
		t.Run(ignoredError.desc, func(t *testing.T) {
			conn, chans := setup(newChannel)
			ignored := ignoredError.err
			ctx := context.Background()

			conn.handleRequests(ctx, nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
				close(chans)
				return ignored
			})

			eventuallyInDelta(t, initialSessionsTotal+2+float64(i), testutil.ToFloat64(metrics.SliSshdSessionsTotal))
			eventuallyInDelta(t, initialSessionsErrorTotal+1, testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal))
		})
	}
}

func eventuallyInDelta(t *testing.T, expected, actual float64) {
	var delta = 0.1
	require.Eventually(t, func() bool {
		return ((expected - actual) < delta)
	}, 1*time.Second, time.Millisecond)
}
