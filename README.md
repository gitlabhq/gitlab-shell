---
stage: Create
group: Source Code
info: To determine the technical writer assigned to the Stage/Group associated with this page, see https://about.gitlab.com/handbook/engineering/ux/technical-writing/#assignments
---

[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

# GitLab Shell

GitLab Shell handles Git SSH sessions for GitLab and modifies the list of
authorized keys. GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

GitLab supports Git LFS authentication through SSH.

## Development Documentation

Development documentation for GitLab Shell [has moved into the `gitlab` repository](https://docs.gitlab.com/ee/development/gitlab_shell/).

## Project structure

| Directory | Description |
|-----------|-------------|
| `cmd/` | 'Commands' that will ultimately be compiled into binaries. |
| `internal/` | Internal Go source code that is not intended to be used outside of the project/module. |
| `client/` | HTTP and GitLab client logic that is used internally and by other modules, e.g. Gitaly. |
| `bin/` | Compiled binaries are created here. |
| `support/` | Scripts and tools that assist in development and/or testing. |

## Building

Run `make` or `make build`.

## Testing

Run `make test`.

## Release Process

1. Create a `gitlab-org/gitlab-shell` MR to update [`VERSION`](https://gitlab.com/gitlab-org/gitlab-shell/-/blob/main/VERSION) and [`CHANGELOG`](https://gitlab.com/gitlab-org/gitlab-shell/-/blob/main/CHANGELOG) files, e.g. [Release v14.39.0](https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/1123).
2. Once `gitlab-org/gitlab-shell` MR is merged, create the corresponding git tag, e.g. https://gitlab.com/gitlab-org/gitlab-shell/-/tags/v14.39.0.
3. Create a `gitlab-org/gitlab` MR to update [`GITLAB_SHELL_VERSION`](https://gitlab.com/gitlab-org/gitlab/-/blob/master/GITLAB_SHELL_VERSION) to the proposed tag, e.g. [Bump GitLab Shell to 14.39.0](https://gitlab.com/gitlab-org/gitlab/-/merge_requests/162661).
4. Announce in `#gitlab-shell` a new version has been created.

## Tag Management Guidelines

This repository is mirrored to security and dev mirrors. Mirror syncs happen within minutes of changes to the canonical repo, so please follow these guidelines when managing tags:

1. **Use caution when deleting and recreating tags** - if a tag has already been synced to the mirrors, recreating it will cause SHA divergence and break the mirror sync.
2. **If a tag needs to be re-created**, coordinate with the release managers first to delete the tag from all mirrors (security + dev) before recreating it on canonical.
3. **If a mirror sync fails due to tag SHA divergence**, contact the release managers in `#g_release_and_deploy` to manually fix the mirrors.

## Licensing

See the `LICENSE` file for licensing information as it pertains to files in
this repository.
