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

1. git pull over ssh  -> gitlab-shell -> API call to gitlab-rails (Authorization) -> accept or decline -> establish Gitaly session
1. git push over ssh  -> gitlab-shell (git command is not executed yet) -> establish Gitaly session -> (in Gitaly) gitlab-shell pre-receive hook -> API call to gitlab-rails (authorization) -> accept or decline push

## Git hooks

For historical reasons the gitlab-shell repository also contains the
Git hooks that allow GitLab to validate Git pushes (e.g. "is this user
allowed to push to this protected branch"). These hooks also trigger
events in GitLab (e.g. to start a CI pipeline after a push). In
GitLab's current architecture (Q4 2018) these hooks belong to Gitaly
more than gitlab-shell. We [are moving them to the Gitaly
repository](https://gitlab.com/gitlab-org/gitaly/issues/1226).

## Code status

[![pipeline status](https://gitlab.com/gitlab-org/gitlab-shell/badges/master/pipeline.svg)](https://gitlab.com/gitlab-org/gitlab-shell/commits/master)
[![coverage report](https://gitlab.com/gitlab-org/gitlab-shell/badges/master/coverage.svg)](https://gitlab.com/gitlab-org/gitlab-shell/commits/master)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

## Requirements

**GitLab shell will always use your system ruby (normally located at /usr/bin/ruby) and will not use the ruby your installed with a ruby version manager (such as RVM).**
It requires ruby 2.0 or higher.
Please uninstall any old ruby versions from your system:

```
sudo apt-get remove ruby1.8
```

Download Ruby and compile it with:

```
mkdir /tmp/ruby && cd /tmp/ruby
curl -L --progress http://cache.ruby-lang.org/pub/ruby/2.1/ruby-2.1.5.tar.gz | tar xz
cd ruby-2.1.5
./configure --disable-install-rdoc
make
sudo make install
```

To install gitlab-shell you also need a Go compiler version 1.8 or newer. https://golang.org/dl/

## Setup

    ./bin/install
    ./bin/compile

## Check

    ./bin/check

## Keys

Add key:

    ./bin/gitlab-keys add-key key-782 "ssh-rsa AAAAx321..."

Remove key:

    ./bin/gitlab-keys rm-key key-23 "ssh-rsa AAAAx321..."

List all keys:

    ./bin/gitlab-keys list-keys


Remove all keys from authorized_keys file:

    ./bin/gitlab-keys clear

## Git LFS remark

Starting with GitLab 8.12, GitLab supports Git LFS authentication through ssh.

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

## Updating VCR fixtures

In order to generate new VCR fixtures you need to have a local GitLab instance 
running and have next data: 

1. gitlab-org/gitlab-test project.
2. SSH key with access to the project and ID 1 that belongs to Administrator.
3. SSH key without access to the project and ID 2.

You also need to modify `secret` variable at `spec/gitlab_net_spec.rb` so tests 
can connect to your local instance.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
