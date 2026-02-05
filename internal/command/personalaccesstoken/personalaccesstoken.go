// Package personalaccesstoken handles operations related to personal access tokens,
// including parsing arguments, requesting tokens, and formatting responses.
package personalaccesstoken

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/personalaccesstoken"
)

const (
	usageText         = "Usage: personal_access_token <name> <scope1[,scope2,...]> [ttl_days]"
	expiresDateFormat = "2006-01-02"
)

// Command represents a command to manage personal access tokens.
type Command struct {
	gitlabClient     *client.GitlabNetClient
	Args             *commandargs.Shell
	ReadWriter       *readwriter.ReadWriter
	TokenArgs        *tokenArgs
	PATAllowedScopes []string
}

type tokenArgs struct {
	Name        string
	Scopes      []string
	ExpiresDate string // Calculated, a TTL is passed from command-line.
}

// Execute processes the command, requests a personal access token, and prints the result.
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	err := c.parseTokenArgs()
	if err != nil {
		return ctx, err
	}

	log.WithContextFields(ctx, log.Fields{
		"token_args": c.TokenArgs,
	}).Info("personalaccesstoken: execute: requesting token")

	response, err := c.getPersonalAccessToken(ctx)
	if err != nil {
		return ctx, err
	}

	_, _ = fmt.Fprint(c.ReadWriter.Out, "Token:   "+response.Token+"\n")
	_, _ = fmt.Fprint(c.ReadWriter.Out, "Scopes:  "+strings.Join(response.Scopes, ",")+"\n")
	_, _ = fmt.Fprint(c.ReadWriter.Out, "Expires: "+response.ExpiresAt+"\n")

	return ctx, nil
}

func (c *Command) parseTokenArgs() error {
	if len(c.Args.SSHArgs) < 3 || len(c.Args.SSHArgs) > 4 {
		return errors.New(usageText) // nolint:stylecheck // usageText is customer facing
	}

	var rectfiedScopes []string
	requestedScopes := strings.Split(c.Args.SSHArgs[2], ",")
	if len(c.PATAllowedScopes) > 0 {
		for _, requestedScope := range requestedScopes {
			if slices.Contains(c.PATAllowedScopes, requestedScope) {
				rectfiedScopes = append(rectfiedScopes, requestedScope)
			}
		}
	} else {
		rectfiedScopes = requestedScopes
	}
	c.TokenArgs = &tokenArgs{
		Name:   c.Args.SSHArgs[1],
		Scopes: rectfiedScopes,
	}

	if len(c.Args.SSHArgs) < 4 {
		c.TokenArgs.ExpiresDate = time.Now().AddDate(0, 0, 30).Format(expiresDateFormat)
		return nil
	}
	rawTTL := c.Args.SSHArgs[3]

	TTL, err := strconv.Atoi(rawTTL)
	if err != nil || TTL < 0 {
		return fmt.Errorf("Invalid value for days_ttl: '%s'", rawTTL) //nolint:stylecheck //message is customer facing
	}

	c.TokenArgs.ExpiresDate = time.Now().AddDate(0, 0, TTL+1).Format(expiresDateFormat)

	return nil
}

func (c *Command) getPersonalAccessToken(ctx context.Context) (*personalaccesstoken.Response, error) {
	client, err := personalaccesstoken.NewClient(c.gitlabClient)
	if err != nil {
		return nil, err
	}

	return client.GetPersonalAccessToken(ctx, c.Args, c.TokenArgs.Name, &c.TokenArgs.Scopes, c.TokenArgs.ExpiresDate)
}
