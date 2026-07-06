// Package sshd provides functionality for SSH daemon connections.
package sshd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"

	"gitlab.com/gitlab-org/labkit/v2/log"
)

const (
	// KeepAliveMsg is the message used for keeping SSH connections alive.
	KeepAliveMsg = "keepalive@openssh.com"

	// sessionChannelType is the SSH channel type for interactive sessions.
	sessionChannelType = "session"

	// NotOurRefError represents the error message indicating that the git upload-pack is not our reference
	NotOurRefError = `exit status 128, stderr: "fatal: git upload-pack: not our ref `

	// brokenPipeError and copyResponseEOF are error-message fragments seen when
	// the client disconnects or aborts a transfer mid-stream. They differ in how
	// they surface, which is why trackError matches them differently:
	//   - brokenPipeError: the git subprocess is killed by SIGPIPE once its
	//     output to the SSH client closes; Gitaly reports this as a gRPC Internal
	//     error, so it is matched only when the code is Internal.
	//   - copyResponseEOF: copying the response back to the client fails with EOF;
	//     this is a plain error at the SSH copy layer with no gRPC code, so it is
	//     matched regardless of status.
	// Both are client-side outcomes, not gitlab-shell/Gitaly failures, so they
	// must not count toward the error SLI.
	//
	// Matching on the message detail is a stopgap. The durable fix is for Gitaly
	// to return Canceled for client disconnects (see the Gitaly issue linked from
	// https://gitlab.com/gitlab-org/gitlab-shell/-/work_items/863), after which
	// the existing grpccodes.Canceled check would cover the broken-pipe case and
	// this match could be simplified.
	brokenPipeError = "signal: broken pipe"
	copyResponseEOF = "copy response: EOF"
)

// EOFTimeout specifies the timeout duration for EOF (End of File) in SSH connections
var EOFTimeout = 10 * time.Second

type connection struct {
	cfg                *config.Config
	concurrentSessions *semaphore.Weighted
	nconn              net.Conn
	maxSessions        int64
	remoteAddr         string
	outcome            connOutcome
}

// connOutcome records, for a single connection, whether authentication was
// attempted and whether any server-side error occurred (at the auth or session
// phase). It feeds the connection-level SLI emitted once in handle().
//
// authAttempted is written only from the auth callback, which ssh.NewServerConn
// invokes inline on handle()'s goroutine, so it needs no synchronization.
// serverError may also be written from per-session goroutines, so it is atomic.
type connOutcome struct {
	authAttempted bool
	serverError   atomic.Bool
}

// observeAuth records the result of a public-key (or certificate) auth attempt.
// A connection that reaches this point spoke SSH and tried to authenticate, so
// it counts toward the connection SLI; port scanners and health checks fail the
// transport handshake earlier and never get here. Only a server-side failure (a
// System *client.APIError, e.g. the internal API was unreachable or redirected)
// marks the connection as an error; client-side failures (unknown key) do not.
func (o *connOutcome) observeAuth(err error) {
	o.authAttempted = true

	// SSH clients may try several keys in sequence, and the final attempt decides
	// the connection's fate. A successful attempt means the connection
	// authenticated, so clear any server-side error recorded by an earlier
	// attempt (e.g. a transient internal API failure that the next key recovered
	// from): the connection ultimately succeeded and must not count as an error.
	// Auth always completes before any session runs, so this never clears a
	// session-phase error.
	if err == nil {
		o.serverError.Store(false)
		return
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.System {
		o.serverError.Store(true)
	}
}

type channelHandler func(context.Context, *ssh.ServerConn, ssh.Channel, <-chan *ssh.Request) error

func newConnection(cfg *config.Config, nconn net.Conn) *connection {
	maxSessions := cfg.Server.ConcurrentSessionsLimit

	return &connection{
		cfg:                cfg,
		maxSessions:        maxSessions,
		concurrentSessions: semaphore.NewWeighted(maxSessions),
		nconn:              nconn,
		remoteAddr:         nconn.RemoteAddr().String(),
	}
}

func (c *connection) handle(ctx context.Context, srvCfg *ssh.ServerConfig, handler channelHandler) {
	log.FromContext(ctx).InfoContext(ctx, "server: handleConn: start")

	// Emit the connection-level SLI once, after the full lifecycle (auth + any
	// sessions) has been observed. handleRequests waits (up to EOFTimeout) for
	// all sessions to be released, so c.outcome normally reflects the final
	// verdict. If EOFTimeout fires with a session still in flight, a late session
	// error may be missed here; that only under-counts errors (never the
	// denominator) in that rare path, and such late errors are typically context
	// cancellations, which trackError excludes anyway.
	defer c.trackConnection()

	sconn, chans, err := c.initServerConn(ctx, srvCfg)
	if err != nil {
		return
	}

	if c.cfg.Server.ClientAliveInterval > 0 {
		ticker := time.NewTicker(time.Duration(c.cfg.Server.ClientAliveInterval))
		defer ticker.Stop()
		go c.sendKeepAliveMsg(ctx, sconn, ticker)
	}

	c.handleRequests(ctx, sconn, chans, handler)

	reason := sconn.Wait()
	if reason != nil {
		log.FromContext(ctx).InfoContext(ctx, "server: handleConn: done", log.ErrorMessage(reason.Error()))
	}
}

func (c *connection) initServerConn(ctx context.Context, srvCfg *ssh.ServerConfig) (*ssh.ServerConn, <-chan ssh.NewChannel, error) {
	if c.cfg.Server.LoginGraceTime > 0 {
		_ = c.nconn.SetDeadline(time.Now().Add(time.Duration(c.cfg.Server.LoginGraceTime)))
		defer func() { _ = c.nconn.SetDeadline(time.Time{}) }()
	}

	sconn, chans, reqs, err := ssh.NewServerConn(c.nconn, srvCfg)
	if err != nil {
		msg := "connection: initServerConn: failed to initialize SSH connection"
		ctx = log.AppendFields(ctx, log.ErrorMessage(err.Error()), slog.String("remote_addr", c.remoteAddr))

		if strings.Contains(err.Error(), "no common algorithm for host key") || err.Error() == "EOF" {
			log.FromContext(ctx).DebugContext(ctx, msg)
		} else {
			log.FromContext(ctx).WarnContext(ctx, msg)
		}

		return nil, nil, err
	}
	go ssh.DiscardRequests(reqs)

	return sconn, chans, err
}

func (c *connection) handleRequests(ctx context.Context, sconn *ssh.ServerConn, chans <-chan ssh.NewChannel, handler channelHandler) {
	requestCtx := log.AppendFields(ctx, slog.String("remote_addr", c.remoteAddr))
	for newChannel := range chans {
		log.FromContext(requestCtx).InfoContext(requestCtx, "connection: handle: new channel requested", slog.String("channel_type", newChannel.ChannelType()))

		if newChannel.ChannelType() != sessionChannelType {
			log.FromContext(requestCtx).InfoContext(requestCtx, "connection: handleRequests: unknown channel type")
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		if !c.concurrentSessions.TryAcquire(1) {
			log.FromContext(requestCtx).InfoContext(requestCtx, "connection: handleRequests: too many concurrent sessions")
			_ = newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			metrics.SshdHitMaxSessions.Inc()
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.FromContext(requestCtx).ErrorContext(requestCtx, "connection: handleRequests: accepting channel failed", log.ErrorMessage(err.Error()))
			c.concurrentSessions.Release(1)
			continue
		}

		go func() {
			defer func(started time.Time) {
				dur := time.Since(started)
				metrics.SshdSessionDuration.Observe(dur.Seconds())
				log.FromContext(requestCtx).InfoContext(requestCtx, "connection: handleRequests: done", log.DurationS(dur))
			}(time.Now())

			defer c.concurrentSessions.Release(1)

			// Prevent a panic in a single session from taking out the whole server
			defer func() {
				if err := recover(); err != nil {
					log.FromContext(requestCtx).ErrorContext(requestCtx, "panic handling session", slog.Any("recovered_error", err))
				}
			}()

			metrics.SliSshdSessionsTotal.Inc()
			err := handler(requestCtx, sconn, channel, requests)
			if err != nil {
				c.trackError(requestCtx, err)
			}
		}()
	}

	// When a connection has been prematurely closed we block execution until all concurrent sessions are released
	// in order to allow Gitaly complete the operations and close all the channels gracefully.
	// If it didn't happen within timeout, we unblock the execution
	// Related issue: https://gitlab.com/gitlab-org/gitlab-shell/-/issues/563
	ctx, cancel := context.WithTimeout(ctx, EOFTimeout)
	defer cancel()
	_ = c.concurrentSessions.Acquire(ctx, c.maxSessions)
}

func (c *connection) sendKeepAliveMsg(ctx context.Context, sconn *ssh.ServerConn, ticker *time.Ticker) {
	ctx = log.AppendFields(ctx, slog.String("remote_addr", c.remoteAddr))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.FromContext(ctx).DebugContext(ctx, "connection: sendKeepAliveMsg: send keepalive message to a client")

			status, payload, err := sconn.SendRequest(KeepAliveMsg, true, nil)
			if err != nil {
				log.FromContext(ctx).ErrorContext(ctx, fmt.Sprintf("Error occurred while sending request :%v", err))
				return
			}

			if status {
				log.FromContext(ctx).DebugContext(ctx, fmt.Sprintf("connection: sendKeepAliveMsg: payload: %v", string(payload)))
			}
		}
	}
}

func (c *connection) trackError(ctx context.Context, err error) {
	// Policy responses from the internal API (e.g. "You are not allowed to
	// push") are expected outcomes and must not count toward the error SLI.
	// System/transport failures (unreachable, followed redirect, 400, or 5xx)
	// indicate a gitlab-shell/infrastructure problem and should.
	var apiError *client.APIError
	if errors.As(err, &apiError) && !apiError.System {
		return
	}

	if errors.Is(err, disallowedcommand.Error) {
		return
	}

	grpcCode := grpcstatus.Code(err)
	if grpcCode == grpccodes.Canceled || grpcCode == grpccodes.Unavailable {
		return
	} else if grpcCode == grpccodes.Internal &&
		(strings.Contains(err.Error(), NotOurRefError) || strings.Contains(err.Error(), brokenPipeError)) {
		return
	}

	// copyResponseEOF arrives as a plain error at the SSH copy layer (no gRPC
	// code), so it is matched regardless of status. A client that disconnects
	// mid-transfer is a client-side outcome, not a gitlab-shell/Gitaly failure,
	// and must not count toward the error SLI.
	if strings.Contains(err.Error(), copyResponseEOF) {
		return
	}

	metrics.SliSshdSessionsErrorsTotal.Inc()
	// Feed the connection-level SLI: a counted session error is a server-side
	// failure for this connection. trackConnection (deferred in handle) emits the
	// connection metrics once, after all sessions have completed.
	c.outcome.serverError.Store(true)
	log.FromContext(ctx).WarnContext(ctx, "connection: session error", log.ErrorMessage(err.Error()))
}

// trackConnection emits the connection-level SLI once per connection. Only
// connections that reached authentication are counted, which excludes port
// scanners, TCP health checks, and protocol-mismatch clients that fail the
// transport handshake before any authentication attempt.
func (c *connection) trackConnection() {
	if !c.outcome.authAttempted {
		return
	}

	metrics.SliSshdConnectionsTotal.Inc()
	if c.outcome.serverError.Load() {
		metrics.SliSshdConnectionsErrorsTotal.Inc()
	}
}
