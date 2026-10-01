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

Releases are automated by the `release:prepare` and `release:tag` CI/CD jobs, which run [`support/release.sh`](support/release.sh):

1. In the latest [`main` pipeline](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main), run the manual `release:prepare` job.
   It opens a `Release vX.Y.Z` merge request, e.g. [Release v14.58.0](https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/1576), that updates
   [`VERSION`](https://gitlab.com/gitlab-org/gitlab-shell/-/blob/main/VERSION) and [`CHANGELOG`](https://gitlab.com/gitlab-org/gitlab-shell/-/blob/main/CHANGELOG)
   with the merge requests merged since the latest tag.
   - The patch version is bumped by default. To bump the minor or major version, set the `RELEASE_BUMP` variable to `minor` or `major` when running the job.
   - To preview the changelog locally, run `support/release.sh changelog`.
2. Review and merge the release merge request. Edit `CHANGELOG` in the merge request if needed, but keep the first line as `vX.Y.Z`.
3. When the merge request is merged, the `release:tag` job in the `main` pipeline creates the `vX.Y.Z` tag.
4. [Renovate](https://gitlab.com/gitlab-org/frontend/renovate-gitlab-bot/-/blob/main/renovate/gitlab/gitlab-shell-version.config.js) opens a `gitlab-org/gitlab` merge request to update [`GITLAB_SHELL_VERSION`](https://gitlab.com/gitlab-org/gitlab/-/blob/master/GITLAB_SHELL_VERSION) to the new tag.
   Review that merge request instead of creating a new one.
5. Announce in `#gitlab-shell` a new version has been created.

Both jobs need the `GITLAB_SHELL_RELEASE_TOKEN` CI/CD variable: a protected and masked project access token with the `api` scope and the Maintainer role.
`CI_JOB_TOKEN` can't be used because it can't create merge requests, and tags created with it don't trigger tag pipelines.

If the jobs can't be used, follow the same steps manually: create the release merge request, create an annotated `vX.Y.Z` tag on its merge commit, and let Renovate bump `GITLAB_SHELL_VERSION`.

## Tag Management Guidelines

This repository is mirrored to security and dev mirrors. Mirror syncs happen within minutes of changes to the canonical repo, so please follow these guidelines when managing tags:

1. **Use caution when deleting and recreating tags** - if a tag has already been synced to the mirrors, recreating it will cause SHA divergence and break the mirror sync.
2. **If a tag needs to be re-created**, [create an issue on the Delivery tracker](https://gitlab.com/gitlab-com/gl-infra/delivery/-/issues/new) and coordinate with the release managers to delete the tag from all mirrors (security + dev) before recreating it on canonical.
3. **If a mirror sync fails due to tag SHA divergence**, contact the release managers in `#g_release_and_deploy` to manually fix the mirrors.

## Licensing

See the `LICENSE` file for licensing information as it pertains to files in
this repository.
