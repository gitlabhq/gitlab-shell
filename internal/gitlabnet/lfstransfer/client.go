package lfstransfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

type Client struct {
	config *config.Config
	args   *commandargs.Shell
	href   string
	auth   string
}

type BatchAction struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

type BatchObject struct {
	Oid           string                  `json:"oid,omitempty"`
	Size          int64                   `json:"size"`
	Authenticated bool                    `json:"authenticated,omitempty"`
	Actions       map[string]*BatchAction `json:"actions,omitempty"`
}

type batchRef struct {
	Name string `json:"name,omitempty"`
}

type batchRequest struct {
	Operation     string         `json:"operation"`
	Objects       []*BatchObject `json:"objects"`
	Ref           *batchRef      `json:"ref,omitempty"`
	HashAlgorithm string         `json:"hash_algo,omitempty"`
}

type BatchResponse struct {
	Objects       []*BatchObject `json:"objects"`
	HashAlgorithm string         `json:"hash_algo,omitempty"`
}

func NewClient(config *config.Config, args *commandargs.Shell, href string, auth string) (*Client, error) {
	return &Client{config: config, args: args, href: href, auth: auth}, nil
}

func (c *Client) Batch(operation string, reqObjects []*BatchObject, ref string, reqHashAlgo string) (*BatchResponse, error) {
	var bref *batchRef

	if ref == "" {
		return nil, errors.New("A ref must be specified.")
	}

	bref = &batchRef{Name: ref}
	body := batchRequest{
		Operation:     operation,
		Objects:       reqObjects,
		Ref:           bref,
		HashAlgorithm: reqHashAlgo,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	jsonReader := bytes.NewReader(jsonData)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/objects/batch", c.href), jsonReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Authorization", c.auth)

	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	response := &BatchResponse{}
	if err := gitlabnet.ParseJSON(res, response); err != nil {
		return nil, err
	}

	return response, nil
}
