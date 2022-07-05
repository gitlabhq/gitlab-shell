package twofactorrecover

import (
	"context"
	"fmt"
	"io"
	"strings"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/twofactorrecover"
)

const readerLimit = 1024

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	ctxlog := log.ContextLogger(ctx)
	ctxlog.Debug("twofactorrecover: execute: Waiting for user input")

	if c.getUserAnswer(ctx) == "yes" {
		ctxlog.Debug("twofactorrecover: execute: User chose to continue")
		c.displayRecoveryCodes(ctx)
	} else {
		ctxlog.Debug("twofactorrecover: execute: User chose not to continue")
		fmt.Fprintln(c.ReadWriter.Out, "\nNew recovery codes have *not* been generated. Existing codes will remain valid.")
	}

	return nil
}

func (c *Command) getUserAnswer(ctx context.Context) string {
	question :=
		"Are you sure you want to generate new two-factor recovery codes?\n" +
			"Any existing recovery codes you saved will be invalidated. (yes/no)"
	fmt.Fprintln(c.ReadWriter.Out, question)

	var answer string
	if _, err := fmt.Fscanln(io.LimitReader(c.ReadWriter.In, readerLimit), &answer); err != nil {
		log.ContextLogger(ctx).WithError(err).Debug("twofactorrecover: getUserAnswer: Failed to get user input")
	}

	return answer
}

func (c *Command) displayRecoveryCodes(ctx context.Context) {
	ctxlog := log.ContextLogger(ctx)

	codes, err := c.getRecoveryCodes(ctx)

	if err == nil {
		ctxlog.Debug("twofactorrecover: displayRecoveryCodes: recovery codes successfully generated")
		messageWithCodes :=
			"\nYour two-factor authentication recovery codes are:\n\n" +
				strings.Join(codes, "\n") +
				"\n\nDuring sign in, use one of the codes above when prompted for\n" +
				"your two-factor code. Then, visit your Profile Settings and add\n" +
				"a new device so you do not lose access to your account again.\n"
		fmt.Fprint(c.ReadWriter.Out, messageWithCodes)
	} else {
		ctxlog.WithError(err).Error("twofactorrecover: displayRecoveryCodes: failed to generate recovery codes")
		fmt.Fprintf(c.ReadWriter.Out, "\nAn error occurred while trying to generate new recovery codes.\n%v\n", err)
	}
}

func (c *Command) getRecoveryCodes(ctx context.Context) ([]string, error) {
	client, err := twofactorrecover.NewClient(c.Config)

	if err != nil {
		return nil, err
	}

	return client.GetRecoveryCodes(ctx, c.Args)
}
