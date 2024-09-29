// Package commandargs provides functionality to handle and parse command-line arguments
// for various GitLab shell commands, including SSH arguments and command types.
package commandargs

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-shellwords"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

// Define supported command types
const (
	Discover            CommandType = "discover"
	TwoFactorRecover    CommandType = "2fa_recovery_codes"
	TwoFactorVerify     CommandType = "2fa_verify"
	LfsAuthenticate     CommandType = "git-lfs-authenticate"
	LfsTransfer         CommandType = "git-lfs-transfer"
	ReceivePack         CommandType = "git-receive-pack"
	UploadPack          CommandType = "git-upload-pack"
	UploadArchive       CommandType = "git-upload-archive"
	PersonalAccessToken CommandType = "personal_access_token"
)

// Regular expressions for parsing key IDs and usernames from arguments
var (
	whoKeyRegex      = regexp.MustCompile(`\Akey-(?P<keyid>\d+)\z`)
	whoUsernameRegex = regexp.MustCompile(`\Ausername-(?P<username>\S+)\z`)

	// List of Git commands that are handled in a special way
	GitCommands = []CommandType{LfsAuthenticate, UploadPack, ReceivePack, UploadArchive}
)

// Shell represents a parsed shell command with its arguments and related information.
type Shell struct {
	Arguments           []string
	GitlabUsername      string
	GitlabKeyID         string
	GitlabKrb5Principal string
	SSHArgs             []string
	CommandType         CommandType
	Env                 sshenv.Env
}

// Parse validates and parses the command-line arguments and SSH environment.
func (s *Shell) Parse() error {
	if err := s.validate(); err != nil {
		return err
	}

	s.parseWho()

	return nil
}

// GetArguments returns the list of command-line arguments.
func (s *Shell) GetArguments() []string {
	return s.Arguments
}

func (s *Shell) validate() error {
	if !s.Env.IsSSHConnection {
		return fmt.Errorf("Only SSH allowed") //nolint:stylecheck //message is customer facing
	}

	if err := s.ParseCommand(s.Env.OriginalCommand); err != nil {
		return fmt.Errorf("Invalid SSH command: %w", err) //nolint:stylecheck //message is customer facing
	}

	return nil
}

func (s *Shell) parseWho() {
	for _, argument := range s.Arguments {
		if keyID := tryParseKeyID(argument); keyID != "" {
			s.GitlabKeyID = keyID
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

func tryParseKeyID(argument string) string {
	return tryParse(whoKeyRegex, argument)
}

func tryParseUsername(argument string) string {
	return tryParse(whoUsernameRegex, argument)
}

// ParseCommand parses the command string into a slice of arguments.
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

	s.SSHArgs = args

	s.defineCommandType()

	return nil
}

func (s *Shell) defineCommandType() {
	if len(s.SSHArgs) == 0 {
		s.CommandType = Discover
	} else {
		s.CommandType = CommandType(s.SSHArgs[0])
	}
}
