### gitlab-shell: ssh access and repostiory management

### Setup

    ./bin/install


### Check 

    ./bin/check


### Repos:
 

Add repo

   ./bin/gitlab-projects add-project gitlab/gitlab-ci.git

Remove repo 

   ./bin/gitlab-projects rm-project gitlab/gitlab-ci.git

### Keys: 


Add key

   ./bin/gitlab-keys add-key key-782 "ssh-rsa AAAAx321..."

Remove key

   ./bin/gitlab-keys rm-key key-23 "ssh-rsa AAAAx321..."

