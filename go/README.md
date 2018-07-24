# Go executables for gitlab-shell

This directory contains Go executables for use in gitlab-shell. To add
a new command `foobar` create a subdirectory `cmd/foobar` and put your
code in `package main` under `cmd/foobar`. This will automatically get
compiled into `bin/foobar` by `../bin/compile`.

## Vendoring

We use vendoring in order to include third-party Go libraries. This
project uses [govendor](https://github.com/kardianos/govendor).

To update e.g. `gitaly-proto` run the following command in the root
directory of the project.

```
support/go-update-vendor gitlab.com/gitlab-org/gitaly-proto/go@v0.109.0
```
