package twofactorverify

import (
	"context"
	"fmt"
	"io"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorverify"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type Result struct {
	Error   error
	Status  string
	Success bool
}

func (c *Command) Execute(ctx context.Context) error {
	verify := make(chan Result)
	pushauth := make(chan Result)

	go func() {
		status, success, err := c.verifyOTP(ctx, c.getOTP())
		verify <- Result{Error: err, Status: status, Success: success}
	}()

	go func() {
		status, success, err := c.pushAuth(ctx)
		pushauth <- Result{Error: err, Status: status, Success: success}
	}()

L:
	for {
		select {
		case res := <-verify:
			if res.Error != nil {
				return res.Error
			}
			fmt.Fprint(c.ReadWriter.Out, res.Status)
			break L
		case res := <-pushauth:
			if res.Success {
				fmt.Fprint(c.ReadWriter.Out, res.Status)
				break L
			} else {
				// ignore reject from remote, need to wait for user input in this case
			}
		}
	}
	return nil
}

func (c *Command) getOTP() string {
	prompt := "OTP: "
	fmt.Fprint(c.ReadWriter.Out, prompt)

	var answer string
	otpLength := int64(64)
	reader := io.LimitReader(c.ReadWriter.In, otpLength)
	fmt.Fscanln(reader, &answer)

	return answer
}

func (c *Command) pushAuth(ctx context.Context) (status string, success bool, err error) {
	client, err := twofactorverify.NewClient(c.Config)
	if err != nil {
		return "", false, err
	}

	reason := ""

	success, reason, err = client.PushAuth(ctx, c.Args)
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

func (c *Command) verifyOTP(ctx context.Context, otp string) (status string, success bool, err error) {
	client, err := twofactorverify.NewClient(c.Config)
	if err != nil {
		return "", false, err
	}

	reason := ""

	success, reason, err = client.VerifyOTP(ctx, c.Args, otp)
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
