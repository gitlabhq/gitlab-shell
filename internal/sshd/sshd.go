package sshd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/authorizedkeys"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
)

const (
	namespace     = "gitlab_shell"
	sshdSubsystem = "sshd"
)

func secondsDurationBuckets() []float64 {
	return []float64{
		0.005, /* 5ms */
		0.025, /* 25ms */
		0.1,   /* 100ms */
		0.5,   /* 500ms */
		1.0,   /* 1s */
		10.0,  /* 10s */
		30.0,  /* 30s */
		60.0,  /* 1m */
		300.0, /* 10m */
	}
}

var (
	sshdConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      "connection_duration_seconds",
			Help:      "A histogram of latencies for connections to gitlab-shell sshd.",
			Buckets:   secondsDurationBuckets(),
		},
	)

	sshdHitMaxSessions = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: sshdSubsystem,
			Name:      "concurrent_limited_sessions_total",
			Help:      "The number of times the concurrent sessions limit was hit in gitlab-shell sshd.",
		},
	)
)

func Run(cfg *config.Config) error {
	authorizedKeysClient, err := authorizedkeys.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize GitLab client: %w", err)
	}

	sshListener, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("failed to listen for connection: %w", err)
	}

	log.Infof("Listening on %v", sshListener.Addr().String())

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() != cfg.User {
				return nil, errors.New("unknown user")
			}
			if key.Type() == ssh.KeyAlgoDSA {
				return nil, errors.New("DSA is prohibited")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			res, err := authorizedKeysClient.GetByKey(ctx, base64.RawStdEncoding.EncodeToString(key.Marshal()))
			if err != nil {
				return nil, err
			}

			return &ssh.Permissions{
				// Record the public key used for authentication.
				Extensions: map[string]string{
					"key-id": strconv.FormatInt(res.Id, 10),
				},
			}, nil
		},
	}

	var loadedHostKeys uint
	for _, filename := range cfg.Server.HostKeyFiles {
		keyRaw, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Warnf("Failed to read host key %v: %v", filename, err)
			continue
		}
		key, err := ssh.ParsePrivateKey(keyRaw)
		if err != nil {
			log.Warnf("Failed to parse host key %v: %v", filename, err)
			continue
		}
		loadedHostKeys++
		config.AddHostKey(key)
	}
	if loadedHostKeys == 0 {
		return fmt.Errorf("No host keys could be loaded, aborting")
	}

	for {
		nconn, err := sshListener.Accept()
		if err != nil {
			log.Warnf("Failed to accept connection: %v\n", err)
			continue
		}

		go handleConn(nconn, config, cfg)
	}
}

type execRequest struct {
	Command string
}

type exitStatusReq struct {
	ExitStatus uint32
}

type envRequest struct {
	Name  string
	Value string
}

func exitSession(ch ssh.Channel, exitStatus uint32) {
	exitStatusReq := exitStatusReq{
		ExitStatus: exitStatus,
	}
	ch.CloseWrite()
	ch.SendRequest("exit-status", false, ssh.Marshal(exitStatusReq))
	ch.Close()
}

func handleConn(nconn net.Conn, sshCfg *ssh.ServerConfig, cfg *config.Config) {
	begin := time.Now()
	defer func() {
		sshdConnectionDuration.Observe(time.Since(begin).Seconds())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer nconn.Close()
	conn, chans, reqs, err := ssh.NewServerConn(nconn, sshCfg)
	if err != nil {
		log.Infof("Failed to initialize SSH connection: %v", err)
		return
	}

	concurrentSessions := semaphore.NewWeighted(cfg.Server.ConcurrentSessionsLimit)

	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		if !concurrentSessions.TryAcquire(1) {
			newChannel.Reject(ssh.ResourceShortage, "too many concurrent sessions")
			sshdHitMaxSessions.Inc()
			continue
		}
		ch, requests, err := newChannel.Accept()
		if err != nil {
			log.Infof("Could not accept channel: %v", err)
			concurrentSessions.Release(1)
			continue
		}

		go handleSession(ctx, concurrentSessions, ch, requests, conn, nconn, cfg)
	}
}

func handleSession(ctx context.Context, concurrentSessions *semaphore.Weighted, ch ssh.Channel, requests <-chan *ssh.Request, conn *ssh.ServerConn, nconn net.Conn, cfg *config.Config) {
	defer concurrentSessions.Release(1)

	rw := &readwriter.ReadWriter{
		Out:    ch,
		In:     ch,
		ErrOut: ch.Stderr(),
	}
	var gitProtocolVersion string

	for req := range requests {
		var execCmd string
		switch req.Type {
		case "env":
			var envRequest envRequest
			if err := ssh.Unmarshal(req.Payload, &envRequest); err != nil {
				ch.Close()
				return
			}
			var accepted bool
			if envRequest.Name == commandargs.GitProtocolEnv {
				gitProtocolVersion = envRequest.Value
				accepted = true
			}
			if req.WantReply {
				req.Reply(accepted, []byte{})
			}

		case "exec":
			var execRequest execRequest
			if err := ssh.Unmarshal(req.Payload, &execRequest); err != nil {
				ch.Close()
				return
			}
			execCmd = execRequest.Command
			fallthrough
		case "shell":
			if req.WantReply {
				req.Reply(true, []byte{})
			}
			args := &commandargs.Shell{
				GitlabKeyId:        conn.Permissions.Extensions["key-id"],
				RemoteAddr:         nconn.RemoteAddr().(*net.TCPAddr),
				GitProtocolVersion: gitProtocolVersion,
			}

			if err := args.ParseCommand(execCmd); err != nil {
				fmt.Fprintf(ch.Stderr(), "Failed to parse command: %v\n", err.Error())
				exitSession(ch, 128)
				return
			}

			cmd := command.BuildShellCommand(args, cfg, rw)
			if cmd == nil {
				fmt.Fprintf(ch.Stderr(), "Unknown command: %v\n", args.CommandType)
				exitSession(ch, 128)
				return
			}
			if err := cmd.Execute(ctx); err != nil {
				fmt.Fprintf(ch.Stderr(), "remote: ERROR: %v\n", err.Error())
				exitSession(ch, 1)
				return
			}
			exitSession(ch, 0)
			return
		default:
			if req.WantReply {
				req.Reply(false, []byte{})
			}
		}
	}
}
