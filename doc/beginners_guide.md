---
stage: Create
group: Source Code
info: To determine the technical writer assigned to the Stage/Group associated with this page, see https://about.gitlab.com/handbook/product/ux/technical-writing/#assignments
---

# Beginner's guide to GitLab Shell contributions

In order to build the binaries a single `make` command can be run:

```shell
make
```

If the command fails due to an error in `gssapi`, make sure that a `Kerberos` implementation is installed. For MacOS it's:

```shell
brew install heimdal
```

It may also require specifying `CGO_CFLAGS`:

```shell
CGO_CFLAGS="-I/opt/homebrew/opt/heimdal/include" make
```

## Check

Checks if GitLab API access and Redis via internal API can be reached:

```shell
make check
```

## Compile

Builds the `gitlab-shell` binaries, placing them into `bin/`.

```shell
make compile
```

## Install

Builds the `gitlab-shell` binaries and installs them onto the file system. The
default location is `/usr/local`, but you can change the location by setting the `PREFIX`
and `DESTDIR` environment variables.

```shell
make install
```

## Setup

This command is intended for use when installing GitLab from source on a single
machine. It compiles the GitLab Shell binaries, and ensures that
various paths on the file system exist with the correct permissions. Do not run
this command unless your installation method documentation instructs you to.

```shell
make setup
```

## Testing

Run tests:

```shell
bundle install
make test
```

Run Gofmt:

```shell
make verify
```

Run both test and verify (the default Makefile target):

```shell
bundle install
make validate
```

## Gitaly

Some tests need a Gitaly server. The
[`docker-compose.yml`](../docker-compose.yml) file runs Gitaly on port 8075.
To tell the tests where Gitaly is, set `GITALY_CONNECTION_INFO`:

```plaintext
export GITALY_CONNECTION_INFO='{"address": "tcp://localhost:8075", "storage": "default"}'
make test
```

If no `GITALY_CONNECTION_INFO` is set, the test suite still runs, but any
tests requiring Gitaly are skipped. The tests always run in the CI environment.

## Logging Guidelines

In general, you can determine the structure, but not content, of a GitLab Shell
or `gitlab-sshd` session by inspecting the logs. Some guidelines:

- We use [`gitlab.com/gitlab-org/labkit/log`](https://pkg.go.dev/gitlab.com/gitlab-org/labkit/log)
  for logging.
- Always include a correlation ID.
- Log messages should be invariant and unique. Include accessory information in
  fields, using `log.WithField`, `log.WithFields`, or `log.WithError`.
- Log both success cases and error cases.
- Logging too much is better than not logging enough. If a message seems too
  verbose, consider reducing the log level before removing the message.
