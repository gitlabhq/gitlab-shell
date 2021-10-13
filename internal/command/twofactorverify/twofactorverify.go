package twofactorverify

import (
	"context"
	"fmt"
	"io"

	"gitlab.com/gitlab-org/labkit/log"

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

func (c *Command) Execute(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)
	ctxlog.Info("twofactorverify: execute: waiting for user input")
	otp := c.getOTP()

	ctxlog.Info("twofactorverify: execute: verifying entered OTP")
	err := c.verifyOTP(ctx, otp)
	if err != nil {
		ctxlog.WithError(err).Error("twofactorverify: execute: OTP verification failed")
		return err
	}

	ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
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

func (c *Command) verifyOTP(ctx context.Context, otp string) error {
	client, err := twofactorverify.NewClient(c.Config)
	if err != nil {
		return err
	}

	err = client.VerifyOTP(ctx, c.Args, otp)
	if err == nil {
		fmt.Fprint(c.ReadWriter.Out, "\nOTP validation successful. Git operations are now allowed.\n")
	} else {
		fmt.Fprintf(c.ReadWriter.Out, "\nOTP validation failed.\n%v\n", err)
	}

	return nil
}
