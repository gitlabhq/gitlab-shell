package command

import (
	"os"
	"regexp"
	"strconv"
)

type CommandType string

const (
	Discover CommandType = "discover"
)

type Command struct {
	GitlabUsername string
	GitlabKeyId    int
	SshConnection  bool
	Command        string
	Type           CommandType
}

func New(arguments []string) (*Command, error) {
	_, sshConnection := os.LookupEnv("SSH_CONNECTION")

	command := &Command{SshConnection: sshConnection}

	if _, err := parseWho(arguments, command); err != nil {
		return nil, err
	}

	originalCommand, _ := os.LookupEnv("SSH_ORIGINAL_COMMAND")
	parseCommand(originalCommand, command)

	return command, nil
}

func parseWho(arguments []string, command *Command) (*Command, error) {
	var err error = nil

	for _, argument := range arguments {
		keyId, err := tryParseKeyId(argument)
		if keyId > 0 && err != nil {
			command.GitlabKeyId = keyId
			break
		}

		username, err := tryParseUsername(argument)
		if username != "" && err != nil {
			command.GitlabUsername = username
			break
		}
	}

	return command, err
}

func tryParseKeyId(argument string) (int, error) {
	whoKeyRegex, err := regexp.Compile(`\bkey-(?P<keyid>\d+)\b`)
	if err != nil {
		return 0, err
	}

	keyMatch := whoKeyRegex.FindString(argument)
	if keyMatch != "" {
		gitlabKeyId, err := strconv.Atoi(keyMatch)

		return gitlabKeyId, err
	}

	return 0, nil
}

func tryParseUsername(argument string) (string, error) {
	whoUsernameRegex, err := regexp.Compile(`\busername-(?P<username>\S+)\b`)
	if err != nil {
		return "", err
	}

	usernameMatch := whoUsernameRegex.FindString(argument)
	return usernameMatch, nil
}

func parseCommand(commandString string, command *Command) *Command {
	command.Command = commandString

	if commandString == "" {
		command.Type = Discover
	}

	return nil
}
