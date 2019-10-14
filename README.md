# GitLab Shell

## GitLab Shell handles git SSH sessions for GitLab

GitLab Shell handles git SSH sessions for GitLab and modifies the list of authorized keys.
GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

When you access the GitLab server over SSH then GitLab Shell will:

1. Limits you to predefined git commands (git push, git pull).
1. Call the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on
1. Copy data back and forth between the SSH client and the Gitaly server

If you access a GitLab server over HTTP(S) you end up in [gitlab-workhorse](https://gitlab.com/gitlab-org/gitlab-workhorse).

An overview of the four cases described above:

1. git pull over SSH -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session
1. git push over SSH -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

## Git hooks

The gitlab-shell repository used to also contain the
Git hooks that allow GitLab to validate Git pushes (e.g. "is this user
allowed to push to this protected branch"). These hooks also trigger
events in GitLab (e.g. to start a CI pipeline after a push).

We are in the process of moving these hooks to Gitaly, because Git hooks
require direct disk access to Git repositories, and that is only
possible on Gitaly servers. It makes no sense to have to install
gitlab-shell on Gitaly servers.

As of GitLab 11.10  [the actual Git hooks are in the Gitaly
repository](https://gitlab.com/gitlab-org/gitaly/tree/v1.22.0/ruby/vendor/gitlab-shell/hooks),
but gitlab-shell must still be installed on Gitaly servers because the
hooks rely on configuration data (e.g. the GitLab internal API URL) that
is not yet available in Gitaly itself. Also see the [transition
plan](https://gitlab.com/gitlab-org/gitaly/issues/1226#note_126519133).

## Code status

[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/master/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/commits/master)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/master/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/commits/master)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

## Requirements

GitLab Shell is written in Go, and needs a Go compiler to build. It still requires
Ruby to build and test, but not to run.

Download and install the current version of Go from https://golang.org/dl/

## Setup

    make setup

## Check

Checks if GitLab API access and redis via internal API can be reached:

    make check

## Testing

Run tests:

    bundle install
    make test

Run gofmt and rubocop:

    bundle install
    make verify

Run both test and verify (the default Makefile target):

    bundle install
    make validate

## Git LFS remark

Starting with GitLab 8.12, GitLab supports Git LFS authentication through SSH.

## Releasing a new version

GitLab Shell is versioned by git tags, and the version used by the Rails
application is stored in
[`GITLAB_SHELL_VERSION`](https://gitlab.com/gitlab-org/gitlab-ce/blob/master/GITLAB_SHELL_VERSION).

For each version, there is a raw version and a tag version:

- The **raw version** is the version number. For instance, `15.2.8`.
- The **tag version** is the raw version prefixed with `v`. For instance, `v15.2.8`.

To release a new version of GitLab Shell and have that version available to the
Rails application:

1. Update the [`CHANGELOG`](CHANGELOG) with the **tag version** and the
   [`VERSION`](VERSION) file with the **raw version**.
2. Add a new git tag with the **tag version**.
3. Update `GITLAB_SHELL_VERSION` in the Rails application to the **raw
   version**. (Note: this can be done as a separate MR to that, or in and MR
   that will make use of the latest GitLab Shell changes.)

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
