### gitlab-shell: ssh access and repository management

#### Code status

* [![CI](http://ci.gitlab.org/projects/4/status?ref=master)](http://ci.gitlab.org/projects/4?ref=master)

* [![Code Climate](https://codeclimate.com/github/gitlabhq/gitlab-shell.png)](https://codeclimate.com/github/gitlabhq/gitlab-shell)

* [![Coverage Status](https://coveralls.io/repos/gitlabhq/gitlab-shell/badge.png?branch=master)](https://coveralls.io/r/gitlabhq/gitlab-shell)


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

    ./bin/gitlab-projects import-project randx/six.git https://github.com/randx/six.git


### Keys: 


Add key

    ./bin/gitlab-keys add-key key-782 "ssh-rsa AAAAx321..."

Remove key

    ./bin/gitlab-keys rm-key key-23 "ssh-rsa AAAAx321..."

