// Package twofactorverify provides functionality for two-factor verification
package twofactorverify

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"gitlab.com/gitlab-org/labkit/fields"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorverify"
)

const (
	timeout = 30 * time.Second
	prompt  = "OTP: "
)

// Command represents the command for two-factor verification
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

// Execute executes the two-factor verification command
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	client, err := twofactorverify.NewClient(c.Config)
	if err != nil {
		return ctx, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, _ = fmt.Fprint(c.ReadWriter.Out, prompt)

	resultCh := make(chan string)
	go func() {
		err := client.PushAuth(ctx, c.Args)
		if err == nil {
			resultCh <- "OTP has been validated by Push Authentication. Git operations are now allowed."
		}
	}()

	go func() {
		answer, err := c.getOTP(ctx)
		if err != nil {
			resultCh <- formatErr(err)
		}

		if err := client.VerifyOTP(ctx, c.Args, answer); err != nil {
			resultCh <- formatErr(err)
		} else {
			resultCh <- "OTP validation successful. Git operations are now allowed."
		}
	}()

	var message string
	select {
	case message = <-resultCh:
	case <-ctx.Done():
		message = formatErr(ctx.Err())
	}

	slog.InfoContext(ctx, "Two factor verify command finished", slog.String("message", message))
	_, _ = fmt.Fprintf(c.ReadWriter.Out, "\n%v\n", message)

	return ctx, nil
}

func (c *Command) getOTP(ctx context.Context) (string, error) {
	var answer string
	otpLength := int64(64)
	reader := io.LimitReader(c.ReadWriter.In, otpLength)
	if _, err := fmt.Fscanln(reader, &answer); err != nil {
		slog.DebugContext(ctx, "twofactorverify: getOTP: Failed to get user input", slog.String(fields.ErrorMessage, err.Error()))
	}

	if answer == "" {
		return "", fmt.Errorf("OTP cannot be blank") //revive:disable:error-strings
	}

	return answer, nil
}

func formatErr(err error) string {
	return fmt.Sprintf("OTP validation failed: %v", err)
}
