---
stage: Create
group: Source Code
info: To determine the technical writer assigned to the Stage/Group associated with this page, see https://about.gitlab.com/handbook/engineering/ux/technical-writing/#assignments
---

# Overview

This package consists of a set of packages that are responsible for executing a particular command/feature/operation.
The full list of features can be viewed [here](https://gitlab.com/gitlab-org/gitlab-shell/-/blob/main/doc/features.md).
The commands implement the common interface:

```go
type Command interface {
	Execute(ctx context.Context) error
}
```

A command is executed by running the `Execute` method. The execution logic mostly shares the common pattern:

- Parse the arguments and validate them.
- Communicate with GitLab Rails using [gitlabnet](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/internal/gitlabnet) package. For example, it can be checking whether a client is authorized to execute this particular command or asking for a personal access token in order to return it to the client.
- If a command is related to Git operations, establish a connection with Gitaly using [handler](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/internal/handler) and [gitaly](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/internal/gitaly) packages and provide two-way communication between Gitaly and the client.
- Return results to the client.

This package is being used to build a particular command based on the passed arguments in the following files that are under `cmd` directory:
- [cmd/gitlab-shell/command](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/cmd/gitlab-shell/command)
- [cmd/check/command](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/cmd/check/command)
- [cmd/gitlab-shell-authorized-keys-check/command](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/cmd/gitlab-shell-authorized-keys-check/command)
- [cmd/gitlab-shell-authorized-principals-check/command](https://gitlab.com/gitlab-org/gitlab-shell/-/tree/main/cmd/gitlab-shell-authorized-principals-check/command)
