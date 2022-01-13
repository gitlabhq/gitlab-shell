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
	timeoutCtx, cancel := context.WithTimeout(ctx, ctxTimeout * time.Second)
	defer cancel()

	// Background push notification with timeout
	pushauth := make(chan Result)
	go func() {
		defer close(pushauth)
		status, success, err := c.pushAuth(timeoutCtx)
		pushauth <- Result{Error: err, Status: status, Success: success}
	}()

	// Also allow manual OTP entry while waiting for push, with same timeout as push
	verify := make(chan Result)
	go func() {
		defer close(verify)
		status, success, err := c.verifyOTP(timeoutCtx, c.getOTP(timeoutCtx))
		verify <- Result{Error: err, Status: status, Success: success}
	}()

	select {
	case res := <-verify: // manual OTP
		fmt.Fprint(c.ReadWriter.Out, res.Status)
	case res := <-pushauth: // push
		fmt.Fprint(c.ReadWriter.Out, res.Status)
	case <-timeoutCtx.Done(): // push timed out
		fmt.Fprint(c.ReadWriter.Out, "OTP verification timed out")
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
