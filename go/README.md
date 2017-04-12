# Go executables for gitlab-shell

This directory contains Go executables for use in gitlab-shell. To add
a new command `foobar` create a subdirectory `cmd/foobar` and put your
code in `package main` under `cmd/foobar`. This will automatically get
compiled into `bin/foobar` by `../bin/compile`.
