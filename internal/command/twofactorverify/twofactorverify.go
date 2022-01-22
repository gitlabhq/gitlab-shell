package twofactorverify

import (
	"context"
	"fmt"
	"io"
	"time"

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
	// config.GetHTTPClient isn't thread-safe so save Client in struct for concurrency
	// workaround until #518 is fixed
	var err error
	c.Client, err = twofactorverify.NewClient(c.Config)
	if err != nil {
		return err
	}

	// Create timeout context 
	// TODO: make timeout configurable
	const ctxTimeout = 30
	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, ctxTimeout * time.Second)
	verifyCtx, cancelVerify := context.WithCancel(timeoutCtx)
	pushCtx, cancelPush := context.WithCancel(timeoutCtx)
	defer cancelTimeout()

	// Background push notification with timeout
	pushauth := make(chan Result)
	go func() {
		defer close(pushauth)
		status, success, err := c.pushAuth(pushCtx)

		select {
		case <-pushCtx.Done(): // push cancelled by manual OTP
			pushauth <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			pushauth <- Result{Error: err, Status: status, Success: success}
			cancelVerify()
		}
	}()

	// Also allow manual OTP entry while waiting for push, with same timeout as push
	verify := make(chan Result)
	go func() {
		defer close(verify)
		answer := ""
		answer = c.getOTP(verifyCtx)

		select {
		case <-verifyCtx.Done(): // manual OTP cancelled by push
			verify <- Result{Error: nil, Status: "cancelled", Success: false}
		default:
			cancelPush()
			status, success, err := c.verifyOTP(verifyCtx, answer)
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
	fmt.Fscanln(reader, &answer)

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
