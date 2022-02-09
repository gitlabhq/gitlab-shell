[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

# GitLab Shell

GitLab Shell is a SSH server, configured to handle Git SSH sessions for GitLab.
GitLab Shell is not a Unix shell, nor a replacement for Bash or Zsh:

- It limits shell access to specific `git` commands.
- It provides authorization and transport for these commands.

## Requirements

GitLab Shell is written in Go, and needs a Go compiler to build. It still requires
Ruby to build and test, but not to run.

Download and install the current version of Go from [golang.org](https://golang.org/dl/)

We follow the [Golang Release Policy](https://golang.org/doc/devel/release.html#policy)
of supporting the current stable version and the previous two major versions.

## Handling `git` SSH sessions

When you access the GitLab server over SSH, GitLab Shell:

1. Limits you to `git push`, `git pull`, and `git fetch` commands only.
1. Calls the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on.
1. Copies data back and forth between the SSH client and the Gitaly server.

### `git pull` over SSH

Git pull over SSH -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session

### `git push` over SSH

Git push over SSH -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

For more details see [Architecture](doc/architecture.md)

### Modifies `authorized_keys`

GitLab Shell modifies the `authorized_keys` file on the client machine.

- TODO some details needed here.

### Runs on port 22

GitLab Shell runs on `port 22` on an Omnibus installation. To use a regular SSH
service, configure it on an alternative port.

### Access GitLab with `https`

If you access a GitLab server over HTTP(S) you end up in
[`gitlab-workhorse`](https://gitlab.com/gitlab-org/gitlab-workhorse).

## `gitlab-sshd`

See [`gitlab-sshd`](doc/gitlab-sshd).

## Commands

- `make check`: Checks if GitLab API access and Redis (via internal API) can be reached
- `make compile`: Builds the `gitlab-shell` binaries, placing them into `bin/`.
- `make install`: Builds the `gitlab-shell` binaries and installs them onto the
  file system. The default location is `/usr/local`, but you can change it with the `PREFIX`
  and `DESTDIR` environment variables.
- `make setup`: Don't run this command unless instructed to by your installation method
  documentation. Used when installing GitLab from source on a single machine. Compiles
  the `gitlab-shell` binaries, and ensures that file system paths exist and contain
  correct permissions.

### Testing

Run tests:

```shell
bundle install
make test
```

Run `gofmt`:

```shell
make verify
```

Run both test and verify (the default Makefile target):

```shell
bundle install
make validate
```

### Gitaly

Some tests need a Gitaly server. The
[`docker-compose.yml`](docker-compose.yml) file runs Gitaly on port 8075.
To tell the tests the location of Gitaly, set `GITALY_CONNECTION_INFO`:

```plaintext
export GITALY_CONNECTION_INFO='{"address": "tcp://localhost:8075", "storage": "default"}'
make test
```

If no `GITALY_CONNECTION_INFO` is set, the test suite still runs, but any
tests requiring Gitaly are skipped. These tests always run in the CI environment.

## References

- [Using the GitLab Shell chart](https://docs.gitlab.com/charts/charts/gitlab/gitlab-shell/#using-the-gitlab-shell-chart)

## Git LFS remark

GitLab supports Git LFS authentication through SSH.

## Releasing

See [PROCESS.md](PROCESS.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

See [LICENSE](LICENSE).
