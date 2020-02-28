package commandargs

import (
	"errors"
	"os"
	"regexp"

	"github.com/mattn/go-shellwords"
)

const (
	Discover         CommandType = "discover"
	TwoFactorRecover CommandType = "2fa_recovery_codes"
	LfsAuthenticate  CommandType = "git-lfs-authenticate"
	ReceivePack      CommandType = "git-receive-pack"
	UploadPack       CommandType = "git-upload-pack"
	UploadArchive    CommandType = "git-upload-archive"

	GitProtocolEnv = "GIT_PROTOCOL"
)

var (
	whoKeyRegex      = regexp.MustCompile(`\bkey-(?P<keyid>\d+)\b`)
	whoUsernameRegex = regexp.MustCompile(`\busername-(?P<username>\S+)\b`)
)

type Shell struct {
	Arguments      []string
	GitlabUsername string
	GitlabKeyId    string
	SshArgs        []string
	CommandType    CommandType
}

func (s *Shell) Parse() error {
	if err := s.validate(); err != nil {
		return err
	}

	s.parseWho()
	s.defineCommandType()

	return nil
}

func (s *Shell) GetArguments() []string {
	return s.Arguments
}

func (s *Shell) validate() error {
	if !s.isSshConnection() {
		return errors.New("Only SSH allowed")
	}

	if !s.isValidSshCommand() {
		return errors.New("Invalid SSH command")
	}

	return nil
}

func (s *Shell) isSshConnection() bool {
	ok := os.Getenv("SSH_CONNECTION")
	return ok != ""
}

func (s *Shell) isValidSshCommand() bool {
	err := s.parseCommand(os.Getenv("SSH_ORIGINAL_COMMAND"))
	return err == nil
}

func (s *Shell) parseWho() {
	for _, argument := range s.Arguments {
		if keyId := tryParseKeyId(argument); keyId != "" {
			s.GitlabKeyId = keyId
			break
		}

		if username := tryParseUsername(argument); username != "" {
			s.GitlabUsername = username
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

func (s *Shell) parseCommand(commandString string) error {
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

	s.SshArgs = args

	return nil
}

func (s *Shell) defineCommandType() {
	if len(s.SshArgs) == 0 {
		s.CommandType = Discover
	} else {
		s.CommandType = CommandType(s.SshArgs[0])
	}
}
