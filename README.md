# GitLab Shell

## GitLab Shell handles git SSH sessions for GitLab

GitLab Shell handles git SSH sessions for GitLab and modifies the list of authorized keys.
GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

When you access the GitLab server over SSH then GitLab Shell will:

1. Limit you to predefined git commands (git push, git pull).
1. Call the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on
1. Copy data back and forth between the SSH client and the Gitaly server

If you access a GitLab server over HTTP(S) you end up in [gitlab-workhorse](https://gitlab.com/gitlab-org/gitlab-workhorse).

An overview of the four cases described above:

1. git pull over SSH -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session
1. git push over SSH -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

## Code status

[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

## Requirements

GitLab Shell is written in Go, and needs a Go compiler to build. It still requires
Ruby to build and test, but not to run.

Download and install the current version of Go from https://golang.org/dl/

We follow the [Golang Release Policy](https://golang.org/doc/devel/release.html#policy)
of supporting the current stable version and the previous two major versions.

## Setup

    make setup

## Check

Checks if GitLab API access and redis via internal API can be reached:

    make check

## Testing

Run tests:

    bundle install
    make test

Run gofmt:

    make verify

Run both test and verify (the default Makefile target):

    bundle install
    make validate

### Gitaly

Some tests need a Gitaly server. The
[`docker-compose.yml`](./docker-compose.yml) file will run Gitaly on
port 8075. To tell the tests where Gitaly is, set
`GITALY_CONNECTION_INFO`:

    export GITALY_CONNECTION_INFO='{"address": "tcp://localhost:8075", "storage": "default"}'
    make test

If no `GITALY_CONNECTION_INFO` is set, the test suite will still run, but any
tests requiring Gitaly will be skipped. They will always run in the CI
environment.

## Git LFS remark

Starting with GitLab 8.12, GitLab supports Git LFS authentication through SSH.

## Releasing

See [PROCESS.md](./PROCESS.md)

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
