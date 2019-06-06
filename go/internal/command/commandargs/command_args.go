package commandargs

import (
	"errors"
	"os"
	"regexp"

	"github.com/mattn/go-shellwords"
)

type CommandType string

const (
	Discover         CommandType = "discover"
	TwoFactorRecover CommandType = "2fa_recovery_codes"
	LfsAuthenticate  CommandType = "git-lfs-authenticate"
	ReceivePack      CommandType = "git-receive-pack"
	UploadPack       CommandType = "git-upload-pack"
	UploadArchive    CommandType = "git-upload-archive"
)

var (
	whoKeyRegex      = regexp.MustCompile(`\bkey-(?P<keyid>\d+)\b`)
	whoUsernameRegex = regexp.MustCompile(`\busername-(?P<username>\S+)\b`)
)

type CommandArgs struct {
	GitlabUsername string
	GitlabKeyId    string
	SshArgs        []string
	CommandType    CommandType
}

func Parse(arguments []string) (*CommandArgs, error) {
	if sshConnection := os.Getenv("SSH_CONNECTION"); sshConnection == "" {
		return nil, errors.New("Only ssh allowed")
	}

	args := &CommandArgs{}
	args.parseWho(arguments)

	if err := args.parseCommand(os.Getenv("SSH_ORIGINAL_COMMAND")); err != nil {
		return nil, errors.New("Invalid ssh command")
	}
	args.defineCommandType()

	return args, nil
}

func (c *CommandArgs) parseWho(arguments []string) {
	for _, argument := range arguments {
		if keyId := tryParseKeyId(argument); keyId != "" {
			c.GitlabKeyId = keyId
			break
		}

		if username := tryParseUsername(argument); username != "" {
			c.GitlabUsername = username
			break
		}
	}
}

func tryParseKeyId(argument string) string {
	matchInfo := whoKeyRegex.FindStringSubmatch(argument)
	if len(matchInfo) == 2 {
		// The first element is the full matched string
		// The second element is the named `keyid`
		return matchInfo[1]
	}

	return ""
}

func tryParseUsername(argument string) string {
	matchInfo := whoUsernameRegex.FindStringSubmatch(argument)
	if len(matchInfo) == 2 {
		// The first element is the full matched string
		// The second element is the named `username`
		return matchInfo[1]
	}

	return ""
}

func (c *CommandArgs) parseCommand(commandString string) error {
	args, err := shellwords.Parse(commandString)
	if err != nil {
		return err
	}

	// Handle Git for Windows 2.14 using "git upload-pack" instead of git-upload-pack
	if len(args) > 1 && args[0] == "git" {
		command := args[0] + "-" + args[1]
		commandArgs := args[2:]

		args = append([]string{command}, commandArgs...)
	}

	c.SshArgs = args

	return nil
}

func (c *CommandArgs) defineCommandType() {
	if len(c.SshArgs) == 0 {
		c.CommandType = Discover
	} else {
		c.CommandType = CommandType(c.SshArgs[0])
	}
}
