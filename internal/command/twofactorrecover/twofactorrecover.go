package twofactorrecover

import (
	"fmt"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/twofactorrecover"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute() error {
	if c.canContinue() {
		c.displayRecoveryCodes()
	} else {
		fmt.Fprintln(c.ReadWriter.Out, "\nNew recovery codes have *not* been generated. Existing codes will remain valid.")
	}

	return nil
}

func (c *Command) canContinue() bool {
	question :=
		"Are you sure you want to generate new two-factor recovery codes?\n" +
			"Any existing recovery codes you saved will be invalidated. (yes/no)"
	fmt.Fprintln(c.ReadWriter.Out, question)

	var answer string
	fmt.Fscanln(c.ReadWriter.In, &answer)

	return answer == "yes"
}

func (c *Command) displayRecoveryCodes() {
	codes, err := c.getRecoveryCodes()

	if err == nil {
		messageWithCodes :=
			"\nYour two-factor authentication recovery codes are:\n\n" +
				strings.Join(codes, "\n") +
				"\n\nDuring sign in, use one of the codes above when prompted for\n" +
				"your two-factor code. Then, visit your Profile Settings and add\n" +
				"a new device so you do not lose access to your account again.\n"
		fmt.Fprint(c.ReadWriter.Out, messageWithCodes)
	} else {
		fmt.Fprintf(c.ReadWriter.Out, "\nAn error occurred while trying to generate new recovery codes.\n%v\n", err)
	}
}

func (c *Command) getRecoveryCodes() ([]string, error) {
	client, err := twofactorrecover.NewClient(c.Config)

	if err != nil {
		return nil, err
	}

	return client.GetRecoveryCodes(c.Args)
}
