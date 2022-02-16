---
stage: Create
group: Source Code
info: To determine the technical writer assigned to the Stage/Group associated with this page, see https://about.gitlab.com/handbook/engineering/ux/technical-writing/#assignments
---

## Beginner's guide to Gitlab Shell contributions

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

### Logging Guidelines

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
