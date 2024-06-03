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

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/personalaccesstoken"
)

const (
	usageText         = "Usage: personal_access_token <name> <scope1[,scope2,...]> [ttl_days]"
	expiresDateFormat = "2006-01-02"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
	TokenArgs  *tokenArgs
}

type tokenArgs struct {
	Name        string
	Scopes      []string
	ExpiresDate string // Calculated, a TTL is passed from command-line.
}

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

	fmt.Fprint(c.ReadWriter.Out, "Token:   "+response.Token+"\n")
	fmt.Fprint(c.ReadWriter.Out, "Scopes:  "+strings.Join(response.Scopes, ",")+"\n")
	fmt.Fprint(c.ReadWriter.Out, "Expires: "+response.ExpiresAt+"\n")

	return ctx, nil
}

func (c *Command) parseTokenArgs() error {
	if len(c.Args.SshArgs) < 3 || len(c.Args.SshArgs) > 4 {
		return errors.New(usageText)
	}

	var rectfiedScopes []string
	requestedScopes := strings.Split(c.Args.SshArgs[2], ",")
	if len(c.Config.PATConfig.AllowedScopes) > 0 {
		for _, requestedScope := range requestedScopes {
			if slices.Contains(c.Config.PATConfig.AllowedScopes, requestedScope) {
				rectfiedScopes = append(rectfiedScopes, requestedScope)
			}
		}
	} else {
		rectfiedScopes = requestedScopes
	}
	c.TokenArgs = &tokenArgs{
		Name:   c.Args.SshArgs[1],
		Scopes: rectfiedScopes,
	}

	if len(c.Args.SshArgs) < 4 {
		c.TokenArgs.ExpiresDate = time.Now().AddDate(0, 0, 30).Format(expiresDateFormat)
		return nil
	}
	rawTTL := c.Args.SshArgs[3]

	TTL, err := strconv.Atoi(rawTTL)
	if err != nil || TTL < 0 {
		return fmt.Errorf("Invalid value for days_ttl: '%s'", rawTTL)
	}

	c.TokenArgs.ExpiresDate = time.Now().AddDate(0, 0, TTL+1).Format(expiresDateFormat)

	return nil
}

func (c *Command) getPersonalAccessToken(ctx context.Context) (*personalaccesstoken.Response, error) {
	client, err := personalaccesstoken.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	return client.GetPersonalAccessToken(ctx, c.Args, c.TokenArgs.Name, &c.TokenArgs.Scopes, c.TokenArgs.ExpiresDate)
}
