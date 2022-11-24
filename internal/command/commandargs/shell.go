package commandargs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-shellwords"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

const (
	Discover            CommandType = "discover"
	TwoFactorRecover    CommandType = "2fa_recovery_codes"
	TwoFactorVerify     CommandType = "2fa_verify"
	LfsAuthenticate     CommandType = "git-lfs-authenticate"
	ReceivePack         CommandType = "git-receive-pack"
	UploadPack          CommandType = "git-upload-pack"
	UploadArchive       CommandType = "git-upload-archive"
	PersonalAccessToken CommandType = "personal_access_token"
)

var (
	whoKeyRegex      = regexp.MustCompile(`\Akey-(?P<keyid>\d+)\z`)
	whoUsernameRegex = regexp.MustCompile(`\Ausername-(?P<username>\S+)\z`)
)

type Shell struct {
	Arguments           []string
	GitlabUsername      string
	GitlabKeyId         string
	GitlabKrb5Principal string
	SshArgs             []string
	CommandType         CommandType
	Env                 sshenv.Env
}

func (s *Shell) Parse() error {
	if err := s.validate(); err != nil {
		return err
	}

	s.parseWho()

	return nil
}

func (s *Shell) GetArguments() []string {
	return s.Arguments
}

func (s *Shell) validate() error {
	if !s.Env.IsSSHConnection {
		return fmt.Errorf("Only SSH allowed")
	}

	if err := s.ParseCommand(s.Env.OriginalCommand); err != nil {
		return fmt.Errorf("Invalid SSH command: %w", err)
	}

	return nil
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

func tryParse(r *regexp.Regexp, argument string) string {
	// sshd may execute the session for AuthorizedKeysCommand in multiple ways:
	// 1. key-id
	// 2. /path/to/shell -c key-id
	args := strings.Split(argument, " ")
	lastArg := args[len(args)-1]

	matchInfo := r.FindStringSubmatch(lastArg)
	if len(matchInfo) == 2 {
		// The first element is the full matched string
		// The second element is the named `keyid` or `username`
		return matchInfo[1]
	}

	return ""
}

func tryParseKeyId(argument string) string {
	return tryParse(whoKeyRegex, argument)
}

func tryParseUsername(argument string) string {
	return tryParse(whoUsernameRegex, argument)
}

func (s *Shell) ParseCommand(commandString string) error {
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

	s.defineCommandType()

	return nil
}

func (s *Shell) defineCommandType() {
	if len(s.SshArgs) == 0 {
		s.CommandType = Discover
	} else {
		s.CommandType = CommandType(s.SshArgs[0])
	}
}
