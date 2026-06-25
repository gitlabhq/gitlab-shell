package sshd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
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

	eventuallyInDelta(t, initialSessionsTotal+1, func() float64 { return testutil.ToFloat64(metrics.SliSshdSessionsTotal) })
	eventuallyInDelta(t, initialSessionsErrorTotal+1, func() float64 { return testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal) })

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

			eventuallyInDelta(t, initialSessionsTotal+2+float64(i), func() float64 { return testutil.ToFloat64(metrics.SliSshdSessionsTotal) })
			eventuallyInDelta(t, initialSessionsErrorTotal+1, func() float64 { return testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal) })
		})
	}
}

func TestSessionErrorMetricDistinguishesAPIErrors(t *testing.T) {
	newChannel := &fakeNewChannel{channelType: sessionChannelType}

	for _, tc := range []struct {
		desc    string
		err     error
		counted bool
	}{
		{
			desc:    "policy API error is not counted",
			err:     &client.APIError{Msg: "You are not allowed to push", StatusCode: 403},
			counted: false,
		},
		{
			desc:    "system API error (redirect misroute) is counted",
			err:     &client.APIError{Msg: `Internal API returned redirect (301) to "http://gitlab.com"`, StatusCode: 301, System: true},
			counted: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			initialErrorTotal := testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal)

			conn, chans := setup(newChannel)
			err := tc.err
			conn.handleRequests(context.Background(), nil, chans, func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error {
				close(chans)
				return err
			})

			expected := initialErrorTotal
			if tc.counted {
				expected = initialErrorTotal + 1
			}
			eventuallyInDelta(t, expected, func() float64 { return testutil.ToFloat64(metrics.SliSshdSessionsErrorsTotal) })
		})
	}
}

func TestConnOutcomeObserveAuth(t *testing.T) {
	t.Run("nil error marks attempted without a server error", func(t *testing.T) {
		var o connOutcome
		o.observeAuth(nil)
		require.True(t, o.authAttempted)
		require.False(t, o.serverError.Load())
	})

	t.Run("system APIError marks a server error", func(t *testing.T) {
		var o connOutcome
		o.observeAuth(&client.APIError{Msg: "redirect", StatusCode: 301, System: true})
		require.True(t, o.authAttempted)
		require.True(t, o.serverError.Load())
	})

	t.Run("policy APIError does not mark a server error", func(t *testing.T) {
		var o connOutcome
		o.observeAuth(&client.APIError{Msg: "You are not allowed", StatusCode: 403})
		require.True(t, o.authAttempted)
		require.False(t, o.serverError.Load())
	})

	t.Run("plain error does not mark a server error", func(t *testing.T) {
		var o connOutcome
		o.observeAuth(errors.New("unknown user"))
		require.True(t, o.authAttempted)
		require.False(t, o.serverError.Load())
	})

	t.Run("a later successful attempt clears an earlier server error", func(t *testing.T) {
		var o connOutcome
		o.observeAuth(&client.APIError{Msg: "redirect", StatusCode: 301, System: true})
		require.True(t, o.serverError.Load())

		// The next key the client offers authenticates successfully.
		o.observeAuth(nil)
		require.True(t, o.authAttempted)
		require.False(t, o.serverError.Load(), "ultimate success must not count as an error")
	})
}

func TestTrackErrorFeedsConnectionOutcome(t *testing.T) {
	t.Run("server-side session error marks the connection outcome", func(t *testing.T) {
		c := &connection{}
		c.trackError(context.Background(), &client.APIError{Msg: "boom", StatusCode: 500, System: true})
		require.True(t, c.outcome.serverError.Load())
	})

	t.Run("client-side session error does not", func(t *testing.T) {
		c := &connection{}
		c.trackError(context.Background(), &client.APIError{Msg: "denied", StatusCode: 403})
		require.False(t, c.outcome.serverError.Load())
	})
}

func TestTrackConnection(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		setup      func(*connOutcome)
		wantTotal  float64
		wantErrors float64
	}{
		{
			desc:       "no auth attempted is not counted (e.g. port scanner / health check)",
			setup:      func(*connOutcome) {},
			wantTotal:  0,
			wantErrors: 0,
		},
		{
			desc:       "auth attempted and succeeded counts as a connection, not an error",
			setup:      func(o *connOutcome) { o.authAttempted = true },
			wantTotal:  1,
			wantErrors: 0,
		},
		{
			desc:       "server-side failure counts as a connection error",
			setup:      func(o *connOutcome) { o.authAttempted = true; o.serverError.Store(true) },
			wantTotal:  1,
			wantErrors: 1,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			initTotal := testutil.ToFloat64(metrics.SliSshdConnectionsTotal)
			initErrors := testutil.ToFloat64(metrics.SliSshdConnectionsErrorsTotal)

			c := &connection{}
			tc.setup(&c.outcome)
			c.trackConnection()

			require.InDelta(t, initTotal+tc.wantTotal, testutil.ToFloat64(metrics.SliSshdConnectionsTotal), 0.01)
			require.InDelta(t, initErrors+tc.wantErrors, testutil.ToFloat64(metrics.SliSshdConnectionsErrorsTotal), 0.01)
		})
	}
}

func eventuallyInDelta(t *testing.T, expected float64, actualFunc func() float64) {
	t.Helper()
	var delta = 0.1
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		actual := actualFunc()
		assert.InDelta(c, expected, actual, delta, "expected: %f, actual: %f", expected, actual)
	}, 5*time.Second, time.Millisecond)
}
