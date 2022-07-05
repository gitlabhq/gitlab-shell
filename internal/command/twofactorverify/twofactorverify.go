package twofactorverify

import (
	"context"
	"fmt"
	"io"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorverify"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)
	ctxlog.Info("twofactorverify: execute: waiting for user input")
	otp := c.getOTP(ctx)

	ctxlog.Info("twofactorverify: execute: verifying entered OTP")
	err := c.verifyOTP(ctx, otp)
	if err != nil {
		ctxlog.WithError(err).Error("twofactorverify: execute: OTP verification failed")
		return err
	}

	ctxlog.WithError(err).Info("twofactorverify: execute: OTP verified")
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
