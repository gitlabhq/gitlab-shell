---
stage: Create
group: Source Code
info: To determine the technical writer assigned to the Stage/Group associated with this page, see https://about.gitlab.com/handbook/engineering/ux/technical-writing/#assignments
---

# GitLab Shell

## GitLab Shell handles git SSH sessions for GitLab

GitLab Shell handles git SSH sessions for GitLab and modifies the list of authorized keys.
GitLab Shell is not a Unix shell nor a replacement for Bash or Zsh.

When you access the GitLab server over SSH then GitLab Shell will:

1. Limit you to predefined git commands (git push, git pull).
1. Call the GitLab Rails API to check if you are authorized, and what Gitaly server your repository is on
1. Copy data back and forth between the SSH client and the Gitaly server

If you access a GitLab server over HTTP(S) you end up in [gitlab-workhorse](https://gitlab.com/gitlab-org/gitlab/tree/master/workhorse).

An overview of the four cases described above:

1. git pull over SSH -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session
1. git push over SSH -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

[Full feature list](doc/features.md)

## Code status

[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/main/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/-/pipelines?ref=main)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

## Requirements

GitLab Shell is written in Go, and needs a Go compiler to build. It still requires
Ruby to build and test, but not to run.

Download and install the current version of Go from https://golang.org/dl/

We follow the [Golang Release Policy](https://golang.org/doc/devel/release.html#policy)
of supporting the current stable version and the previous two major versions.

## Rate Limiting

GitLab Shell performs rate-limiting by user account and project for git operations. GitLab Shell accepts git operation requests and then makes a call to the Rails rate-limiter (backed by Redis). If the `user + project` exceeds the rate limit then GitLab Shell will then drop further connection requests for that `user + project`.

The rate-limiter is applied at the git command (plumbing) level. Each command has a rate limit of 600/minute. For example, `git push` has 600/minute and `git pull` has another 600/minute.

Because they are using the same plumbing command `git-upload-pack`, `git pull` and `git clone` are in effect the same command for the purposes of rate-limiting.

There is also a rate-limiter in place in Gitaly, but the calls will never be made to Gitaly if the rate limit is exceeded in Gitlab Shell (Rails).

## GitLab SaaS

A diagram of the flow of `gitlab-shell` on GitLab.com:

```mermaid
graph LR
    a2 --> b2
    a2  --> b3
    a2 --> b4
    b2 --> c1
    b3 --> c1
    b4 --> c1
    c2 --> d1
    c2 --> d2
    c2 --> d3
    d1 --> e1
    d2 --> e1
    d3 --> e1
    a1[Cloudflare] --> a2[TCP<br/> load balancer]
    e1[Git]

    subgraph HAProxy Fleet
    b2[HAProxy]
    b3[HAProxy]
    b4[HAProxy]
    end

    subgraph GKE
    c1[Internal TCP<br/> load balancer<br/>port 2222] --> c2[GitLab-shell<br/> pods]
    end

    subgraph Gitaly
    d1[Gitaly]
    d2[Gitaly]
    d3[Gitaly]
    end
```

## Releasing

See [PROCESS.md](./PROCESS.md)

## Contributing

- See [CONTRIBUTING.md](./CONTRIBUTING.md).
- See the [beginner's guide](doc/beginners_guide.md).

## License

See [LICENSE](./LICENSE).
