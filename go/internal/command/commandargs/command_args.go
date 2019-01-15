package commandargs

import (
	"errors"
	"os"
	"regexp"
)

type CommandType string

const (
	Discover CommandType = "discover"
)

var (
	whoKeyRegex      = regexp.MustCompile(`\bkey-(?P<keyid>\d+)\b`)
	whoUsernameRegex = regexp.MustCompile(`\busername-(?P<username>\S+)\b`)
)

type CommandArgs struct {
	GitlabUsername string
	GitlabKeyId    string
	SshCommand     string
	CommandType    CommandType
}

func Parse(arguments []string) (*CommandArgs, error) {
	if sshConnection := os.Getenv("SSH_CONNECTION"); sshConnection == "" {
		return nil, errors.New("Only ssh allowed")
	}

	info := &CommandArgs{}

	info.parseWho(arguments)
	info.parseCommand(os.Getenv("SSH_ORIGINAL_COMMAND"))

	return info, nil
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

func (c *CommandArgs) parseCommand(commandString string) {
	c.SshCommand = commandString

	if commandString == "" {
		c.CommandType = Discover
	}
}
