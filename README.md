# gitlab-shell: ssh access and repository management

GitLab Shell is an application that allows you to execute git commands and provide ssh access to git repositories. It is not a unix shell nor a replacement for Bash or Zsh.

#### Code status

[![CI](http://ci.gitlab.org/projects/4/status.png?ref=master)](http://ci.gitlab.org/projects/4?ref=master)
[![Build Status](https://travis-ci.org/gitlabhq/gitlab-shell.png?branch=master)](https://travis-ci.org/gitlabhq/gitlab-shell)
[![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.png)](https://codeclimate.com/github/gitlabhq/gitlab-shell)
[![Coverage Status](https://coveralls.io/repos/gitlabhq/gitlab-shell/badge.png?branch=master)](https://coveralls.io/r/gitlabhq/gitlab-shell)


__Requires ruby 1.9+__


### Setup

    ./bin/install


### Check 

    ./bin/check


### Repos:
 

Add repo

    ./bin/gitlab-projects add-project gitlab/gitlab-ci.git

Remove repo 

    ./bin/gitlab-projects rm-project gitlab/gitlab-ci.git

Import repo 

    # Default timeout is 2 minutes 
    ./bin/gitlab-projects import-project randx/six.git https://github.com/randx/six.git 
    
    # Override timeout in seconds
    ./bin/gitlab-projects import-project randx/six.git https://github.com/randx/six.git 90

Fork repo

    ./bin/gitlab-projects fork-project gitlab/gitlab-ci.git randx

Update HEAD

    ./bin/gitlab-projects update-head gitlab/gitlab-ci.git 3-2-stable

Create branch

    ./bin/gitlab-projects create-branch gitlab/gitlab-ci.git 3-2-stable master

Remove branch

    ./bin/gitlab-projects rm-branch gitlab/gitlab-ci.git 3-0-stable

Create tag (lightweight & annotated)

    ./bin/gitlab-projects create-tag gitlab/gitlab-ci.git v3.0.0 3-0-stable 
    ./bin/gitlab-projects create-tag gitlab/gitlab-ci.git v3.0.0 3-0-stable 'annotated message goes here'

Remove tag

    ./bin/gitlab-projects rm-tag gitlab/gitlab-ci.git v3.0.0


### Keys: 


Add key

    ./bin/gitlab-keys add-key key-782 "ssh-rsa AAAAx321..."

Remove key

    ./bin/gitlab-keys rm-key key-23 "ssh-rsa AAAAx321..."

Remove all keys from authorized_keys file

    ./bin/gitlab-keys clear

