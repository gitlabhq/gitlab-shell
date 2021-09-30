[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

# GitLab Shell

GitLab Shell is a SSH server, configured to handing git SSH sessions for GitLab. GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

The purpose of GitLab Shell is to limit shell access to specific `git` commands, and provide authorization and transport for these commands.


## Handling `git` SSH sessions

When you access the GitLab server over SSH then GitLab Shell will:

1. Limit you to `git push` and `git pull`, `git fetch` commands only
1. Call the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on
1. Copy data back and forth between the SSH client and the Gitaly server

### `git pull` over SSH

git pull over SSH -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session

### `git push` over SSH

git push over SSH -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

For more details see [Architecture](doc/architecture.md)

### Modifies `authorized_keys`

GitLab Shell modifies the `authorized_keys` file on the client machine.

- TODO some details needed here.

### Runs on Port 22

GitLab Shell runs on `port 22` on an Omnibus installation. A "regular" SSH service would need to be configured to run on an alternative port.

### Accessing GitLab using `https`

If you access a GitLab server over HTTP(S) you end up in [gitlab-workhorse](https://gitlab.com/gitlab-org/gitlab-workhorse).

## `gitlab-sshd`

See [gitlab-sshd](doc/gitlab-sshd)

## Requirements

GitLab Shell is written in Go, and needs a Go compiler to build. It still requires
Ruby to build and test, but not to run.

Download and install the current version of Go from https://golang.org/dl/

We follow the [Golang Release Policy](https://golang.org/doc/devel/release.html#policy)
of supporting the current stable version and the previous two major versions.

### Check

Checks if GitLab API access and redis via internal API can be reached:

    make check

### Compile

Builds the `gitlab-shell` binaries, placing them into `bin/`.

    make compile

### Install

Builds the `gitlab-shell` binaries and installs them onto the filesystem. The
default location is `/usr/local`, but can be controlled by use of the `PREFIX`
and `DESTDIR` environment variables.

    make install

### Setup

This command is intended for use when installing GitLab from source on a single
machine. In addition to compiling the gitlab-shell binaries, it ensures that
various paths on the filesystem exist with the correct permissions. Do not run
it unless instructed to by your installation method documentation.

    make setup


### Testing

Run tests:

    bundle install
    make test

Run `gofmt`:

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

## References

- [Using the GitLab Shell chart](https://docs.gitlab.com/charts/charts/gitlab/gitlab-shell/#using-the-gitlab-shell-chart)

## Git LFS remark

GitLab supports Git LFS authentication through SSH.

## Releasing

See [PROCESS.md](./PROCESS.md)

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
