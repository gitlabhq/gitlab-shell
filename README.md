[![build status](https://gitlab.com/gitlab-org/gitlab-shell/badges/master/build.svg)](https://gitlab.com/gitlab-org/gitlab-shell/commits/master)
[![Build Status](https://semaphoreapp.com/api/v1/projects/a71ddd46-a9cc-4062-875e-7ade19a44927/243336/badge.svg)](https://semaphoreapp.com/gitlabhq/gitlab-shell)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.svg)](https://codeclimate.com/github/gitlabhq/gitlab-shell)
[![Coverage Status](https://coveralls.io/repos/gitlabhq/gitlab-shell/badge.svg?branch=master)](https://coveralls.io/r/gitlabhq/gitlab-shell)

# GitLab Shell

GitLab Shell handles git commands for GitLab and modifies the server's list of 
authorized keys. It is not a Unix shell nor a replacement for Bash or Zsh.

When predefined git commands (`git push`, `git pull`, `git annex`) are passed to 
to the server over `ssh`, GitLab Shell will:

1. Call the GitLab Rails API to check if you are authorized
1. Execute the pre-receive hooks (called "Git Hooks" in GitLab Enterprise Edition)
1. Execute the action you requested
1. Process the GitLab post-receive actions
1. Process any custom post-receive actions

These steps are carried out differently for push and pull requests made over http(s):

**When you pull from a git repository over http(s)**, the GitLab Rails app handles 
authentication and execution entirely on its own.

**When you push to a git repository over http(s)**, the GitLab Rails app first 
delegates authentication and execution to GitLab Shell.

This is because GitLab Rails doesn't have logic for interpreting git push 
commands over http(s). This logic is kept in one place (GitLab Shell) so it can 
be reused for commands passed over ssh.

Similarly, GitLab Shell does not have logic for conducting authorization. GitLab 
Shell executes all push commands before conducting authorization, relying on 
pre-receive hooks to do so by triggering API calls to gitlab-rails to check 
authorization. 

Steps for these four modes of access are represented here:

- **pull over ssh**  -> received by gitlab-shell -> API call to gitlab-rails (authorization) -> accept or decline -> git command executed
- **pull over http(s)** -> received by gitlab-rails -> authorization handled internally by gitlab-rails -> accept or decline -> git command executed

- **push over ssh**  -> received by gitlab-shell -> git command executed -> gitlab-shell pre-receive hook invoked -> API call to gitlab-rails (authorization) -> accept or decline push
- **push over http(s)** -> received by gitlab-rails -> git command executed -> gitlab-shell pre-receive hook invoked -> API call to gitlab-rails (authorization) -> accept or decline push

## System Requirements

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
## Shell Commands

### Setup

    ./bin/install

### Check

    ./bin/check

### Repos

Add repo:

    ./bin/gitlab-projects add-project gitlab/gitlab-ci.git

Remove repo:

    ./bin/gitlab-projects rm-project gitlab/gitlab-ci.git

List repos:

    ./bin/gitlab-projects list-projects

Import repo:

    # Default timeout is 2 minutes
    ./bin/gitlab-projects import-project randx/six.git https://github.com/randx/six.git

    # Override timeout in seconds
    ./bin/gitlab-projects import-project randx/six.git https://github.com/randx/six.git 90

Fork repo:

    ./bin/gitlab-projects fork-project gitlab/gitlab-ci.git randx

Create tag (lightweight & annotated):

    ./bin/gitlab-projects create-tag gitlab/gitlab-ci.git v3.0.0 3-0-stable
    ./bin/gitlab-projects create-tag gitlab/gitlab-ci.git v3.0.0 3-0-stable 'annotated message goes here'

Gc repo:

    ./bin/gitlab-projects gc gitlab/gitlab-ci.git

### Keys

Add key:

    ./bin/gitlab-keys add-key key-782 "ssh-rsa AAAAx321..."

Remove key:

    ./bin/gitlab-keys rm-key key-23 "ssh-rsa AAAAx321..."

List all keys:

    ./bin/gitlab-keys list-keys


Remove all keys from authorized_keys file:

    ./bin/gitlab-keys clear

## A Note about Git LFS

Starting with GitLab 8.12, GitLab supports Git LFS authentication through ssh.
