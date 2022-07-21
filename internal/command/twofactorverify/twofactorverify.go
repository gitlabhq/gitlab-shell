package twofactorverify

import (
	"context"
	"fmt"
	"io"
	"time"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorverify"
)

const (
	timeout = 30 * time.Second
	prompt  = "OTP: "
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	client, err := twofactorverify.NewClient(c.Config)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fmt.Fprint(c.ReadWriter.Out, prompt)

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

	log.WithContextFields(ctx, log.Fields{"message": message}).Info("Two factor verify command finished")
	fmt.Fprintf(c.ReadWriter.Out, "\n%v\n", message)

	return nil
}

func (c *Command) getOTP(ctx context.Context) (string, error) {
	var answer string
	otpLength := int64(64)
	reader := io.LimitReader(c.ReadWriter.In, otpLength)
	if _, err := fmt.Fscanln(reader, &answer); err != nil {
		log.ContextLogger(ctx).WithError(err).Debug("twofactorverify: getOTP: Failed to get user input")
	}

	if answer == "" {
		return "", fmt.Errorf("OTP cannot be blank.")
	}

	return answer, nil
}

func formatErr(err error) string {
	return fmt.Sprintf("OTP validation failed: %v", err)
}
