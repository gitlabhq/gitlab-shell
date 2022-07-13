package twofactorverify

import (
	"context"
	"fmt"
	"io"
	"time"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorverify"
)

type Command struct {
	Config     *config.Config
	Client     *twofactorverify.Client
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type Result struct {
	Error   error
	Status  string
	Success bool
}

func (c *Command) Execute(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)

	// config.GetHTTPClient isn't thread-safe so save Client in struct for concurrency
	// workaround until #518 is fixed
	var err error
	c.Client, err = twofactorverify.NewClient(c.Config)

	if err != nil {
		ctxlog.WithError(err).Error("twofactorverify: execute: OTP verification failed")
		return err
	}

	// Create timeout context
	// TODO: make timeout configurable
	const ctxTimeout = 30
	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, ctxTimeout*time.Second)
	defer cancelTimeout()

	// Background push notification with timeout
	pushauth := make(chan Result)
	go func() {
		defer close(pushauth)
		status, success, err := c.pushAuth(timeoutCtx)

		select {
		case <-timeoutCtx.Done(): // push cancelled by manual OTP
			pushauth <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			pushauth <- Result{Error: err, Status: status, Success: success}
			cancelTimeout()
		}
	}()

	// Also allow manual OTP entry while waiting for push, with same timeout as push
	verify := make(chan Result)
	go func() {
		defer close(verify)
		ctxlog.Info("twofactorverify: execute: waiting for user input")
		answer := ""
		answer = c.getOTP(timeoutCtx)

		select {
		case <-timeoutCtx.Done(): // manual OTP cancelled by push
			verify <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			cancelTimeout()
			ctxlog.Info("twofactorverify: execute: verifying entered OTP")
			status, success, err := c.verifyOTP(timeoutCtx, answer)
			ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
			verify <- Result{Error: err, Status: status, Success: success}
		}
	}()

	for {
		select {
		case res := <-verify: // manual OTP
			if res.Status == "cancelled" {
				// verify cancelled; don't print anything
			} else if res.Status == "" {
				// channel closed; don't print anything
			} else {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				return nil
			}
		case res := <-pushauth: // push
			if res.Status == "cancelled" {
				// push cancelled; don't print anything
			} else if res.Status == "" {
				// channel closed; don't print anything
			} else {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				return nil
			}
		case <-timeoutCtx.Done(): // push timed out
			fmt.Fprint(c.ReadWriter.Out, "\nOTP verification timed out\n")
			return nil
		}
	}

	return nil
}

func (c *Command) getOTP(ctx context.Context) string {
	prompt := "OTP: "
	fmt.Fprint(c.ReadWriter.Out, prompt)

	var answer string
	otpLength := int64(64)
	reader := io.LimitReader(c.ReadWriter.In, otpLength)
	if _, err := fmt.Fscanln(reader, &answer); err != nil {
		log.ContextLogger(ctx).WithError(err).Debug("twofactorverify: getOTP: Failed to get user input")
	}

	return answer
}

func (c *Command) verifyOTP(ctx context.Context, otp string) (status string, success bool, err error) {
	reason := ""

	success, reason, err = c.Client.VerifyOTP(ctx, c.Args, otp)
	if success {
		status = fmt.Sprintf("\nOTP validation successful. Git operations are now allowed.\n")
	} else {
		if err != nil {
			status = fmt.Sprintf("\nOTP validation failed.\n%v\n", err)
		} else {
			status = fmt.Sprintf("\nOTP validation failed.\n%v\n", reason)
		}
	}

	err = nil

	return
}

func (c *Command) pushAuth(ctx context.Context) (status string, success bool, err error) {
	reason := ""

	success, reason, err = c.Client.PushAuth(ctx, c.Args)
	if success {
		status = fmt.Sprintf("\nPush OTP validation successful. Git operations are now allowed.\n")
	} else {
		if err != nil {
			status = fmt.Sprintf("\nPush OTP validation failed.\n%v\n", err)
		} else {
			status = fmt.Sprintf("\nPush OTP validation failed.\n%v\n", reason)
		}
	}

	return
}
