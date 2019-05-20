package twofactorrecover

import (
	"fmt"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/twofactorrecover"
)

type Command struct {
	Config *config.Config
	Args   *commandargs.CommandArgs
}

func (c *Command) Execute(readWriter *readwriter.ReadWriter) error {
	if c.canContinue(readWriter) {
		c.displayRecoveryCodes(readWriter)
	} else {
		fmt.Fprintln(readWriter.Out, "\nNew recovery codes have *not* been generated. Existing codes will remain valid.")
	}

	return nil
}

func (c *Command) canContinue(readWriter *readwriter.ReadWriter) bool {
	question :=
		"re you sure you want to generate new two-factor recovery codes?\n" +
			"Any existing recovery codes you saved will be invalidated. (yes/no)"
	fmt.Fprintln(readWriter.Out, question)

	var answer string
	fmt.Fscanln(readWriter.In, &answer)

	return answer == "yes"
}

func (c *Command) displayRecoveryCodes(readWriter *readwriter.ReadWriter) {
	codes, err := c.getRecoveryCodes()

	if err == nil {
		messageWithCodes :=
			"\nYour two-factor authentication recovery codes are:\n\n" +
				strings.Join(codes, "\n") +
				"\n\nDuring sign in, use one of the codes above when prompted for\n" +
				"your two-factor code. Then, visit your Profile Settings and add\n" +
				"a new device so you do not lose access to your account again.\n"
		fmt.Fprint(readWriter.Out, messageWithCodes)
	} else {
		fmt.Fprintf(readWriter.Out, "\nAn error occurred while trying to generate new recovery codes.\n%v\n", err)
	}
}

func (c *Command) getRecoveryCodes() ([]string, error) {
	client, err := twofactorrecover.NewClient(c.Config)

	if err != nil {
		return nil, err
	}

	return client.GetRecoveryCodes(c.Args)
}
