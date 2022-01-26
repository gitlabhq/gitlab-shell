# GitLab Shell

## GitLab Shell handles git SSH sessions for GitLab

GitLab Shell handles git SSH sessions for GitLab and modifies the list of authorized keys.
GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

When you access the GitLab server over SSH then GitLab Shell will:

1. Limit you to predefined git commands (git push, git pull).
1. Call the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on
1. Copy data back and forth between the SSH client and the Gitaly server

If you access a GitLab server over HTTP(S) you end up in [gitlab-workhorse](https://gitlab.com/gitlab-org/gitlab/tree/master/workhorse).

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

## Check

Checks if GitLab API access and redis via internal API can be reached:

    make check

## Compile

Builds the `gitlab-shell` binaries, placing them into `bin/`.

    make compile

## Install

Builds the `gitlab-shell` binaries and installs them onto the filesystem. The
default location is `/usr/local`, but can be controlled by use of the `PREFIX`
and `DESTDIR` environment variables.

    make install

## Setup

This command is intended for use when installing GitLab from source on a single
machine. In addition to compiling the gitlab-shell binaries, it ensures that
various paths on the filesystem exist with the correct permissions. Do not run
it unless instructed to by your installation method documentation.

    make setup


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

## Git LFS

Starting with GitLab 8.12, GitLab supports Git LFS authentication through SSH.

## Logging Guidelines

In general, it should be possible to determine the structure, but not content,
of a gitlab-shell or gitlab-sshd session just from inspecting the logs. Some
guidelines:

- We use [`gitlab.com/gitlab-org/labkit/log`](https://pkg.go.dev/gitlab.com/gitlab-org/labkit/log)
  for logging functionality
- **Always** include a correlation ID
- Log messages should be invariant and unique. Include accessory information in
  fields, using `log.WithField`, `log.WithFields`, or `log.WithError`.
- Log success cases as well as error cases
- Logging too much is better than not logging enough. If a message seems too
  verbose, consider reducing the log level before removing the message.

## Rate Limiting

GitLab Shell performs rate-limiting by user account and project for git operations. GitLab Shell accepts git operation requests and then makes a call to the Rails rate-limiter (backed by Redis). If the `user + project` exceeds the rate limit then GitLab Shell will then drop further connection requests for that `user + project`.

The rate-limiter is applied at the git command (plumbing) level. Each command has a rate limit of 600/minute. For example, `git push` has 600/minute and `git pull` has another 600/minute. 

Because they are using the same plumbing command `git-upload-pack`, `git pull` and `git clone` are in effect the same command for the purposes of rate-limiting.

There is also a rate-limiter in place in Gitaly, but the calls will never be made to Gitaly if the rate limit is exceeded in Gitlab Shell (Rails).

## Releasing

See [PROCESS.md](./PROCESS.md)

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
