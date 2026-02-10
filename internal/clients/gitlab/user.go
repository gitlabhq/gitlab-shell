package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

var (
	ErrInvalidWho = errors.New("who='' is invalid")
)

// Response represents the response structure for user discovery
type User struct {
	UserID   int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type GetUserArgs struct {
	GitlabUsername      string
	GitlabKeyID         string
	GitlabKrb5Principal string
}

// GetUser retrieves user information based on command arguments
func (c *Client) GetUser(ctx context.Context, args GetUserArgs) (*User, error) {
	params := url.Values{}
	switch {
	case args.GitlabUsername != "":
		params.Add("username", args.GitlabUsername)
	case args.GitlabKeyID != "":
		params.Add("key_id", args.GitlabKeyID)
	case args.GitlabKrb5Principal != "":
		params.Add("krb5principal", args.GitlabKrb5Principal)
	default:
		// There was no 'who' information, this matches the ruby error
		// message.
		return nil, ErrInvalidWho
	}

	return c.getUserResponse(ctx, params)
}

func (c *Client) getUserResponse(ctx context.Context, params url.Values) (*User, error) {
	path := "/discover?" + params.Encode()

	resp, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	user := &User{}
	if err := gitlabnet.ParseJSON(resp, user); err != nil {
		return nil, err
	}

	return user, nil
}

// IsAnonymous checks if the user is anonymous
func (u *User) IsAnonymous() bool {
	return u.UserID < 1
}
