#!/usr/bin/env bash

## Container startup script

### Setup sshd configuration
LOG_LEVEL=${LOG_LEVEL:-INFO}

echo "Setting sshd LogLevel to $LOG_LEVEL"

chmod -R o-rwx $GIT_REPO_ROOT
chmod 700 $GIT_REPO_ROOT

sed -i "s/#LogLevel INFO/LogLevel ${LOG_LEVEL:-INFO}/" /etc/ssh/sshd_config

### Setup keys
SSH_SECRETS_DIR=${SSH_SECRETS_DIR:-/etc/feedhenry/gitlab-shell}
SSH_FOLDER=/home/git/.ssh
AUTHORIZED_KEYS_FILE=$SSH_FOLDER/authorized_keys

## Mounted volume for git repositories
GIT_REPO_MOUNT=${GIT_REPO_MOUNT:-/home/git/data}

## Path where git repositories are stored
GIT_REPO_ROOT=${GIT_REPO_ROOT:-/home/git/data/gls}

echo "Initializing authorized_keys file"

##### Store authorized_keys on persisted volume for backup
mkdir -p $GIT_REPO_ROOT/.ssh/ /home/git/.ssh/
touch -a $GIT_REPO_ROOT/.ssh/authorized_keys

ln -sf $GIT_REPO_ROOT/.ssh/authorized_keys $AUTHORIZED_KEYS_FILE

SSH_CMD_PREFIX='command="export GL_ID=key-gitlabshelladmin;if [ -n \"$SSH_ORIGINAL_COMMAND\" ]; then eval \"$SSH_ORIGINAL_COMMAND\";else exec \"$SHELL\"; fi" '

PUB_KEY="gitlab-shell-id-rsa-pub"
FILE_CONTENTS=`cat $SSH_SECRETS_DIR/$PUB_KEY`
if grep -q "$FILE_CONTENTS" $AUTHORIZED_KEYS_FILE; then
  echo "$PUB_KEY key already exist in authorized_keys file."
else
  if [ -f $SSH_SECRETS_DIR/$PUB_KEY ]; then
     echo "Adding $SSH_SECRETS_DIR/$PUB_KEY to $AUTHORIZED_KEYS_FILE"
     echo "$SSH_CMD_PREFIX$(< $SSH_SECRETS_DIR/$PUB_KEY)" >> $AUTHORIZED_KEYS_FILE
  else
     echo "WARN: $SSH_SECRETS_DIR/$PUB_KEY file missing!"
  fi
fi

PUB_KEY="repoadmin-id-rsa-pub"
FILE_CONTENTS=`cat $SSH_SECRETS_DIR/$PUB_KEY`
if grep -q "$FILE_CONTENTS" $AUTHORIZED_KEYS_FILE; then
  echo "$PUB_KEY key already exist in authorized_keys file."
else
  if [ -f $SSH_SECRETS_DIR/$PUB_KEY ]; then
     echo "Adding $SSH_SECRETS_DIR/$PUB_KEY to $AUTHORIZED_KEYS_FILE"
     ./bin/gitlab-keys add-key key-repoadmin "$(< $SSH_SECRETS_DIR/$PUB_KEY)"
  else
     echo "WARN: $SSH_SECRETS_DIR/$PUB_KEY file missing!"
  fi
fi

# Enable scl ruby package
echo "source /opt/rh/rh-ruby23/enable" > /home/git/.bashrc

## Run gitlab setup
./bin/install

## Map PV repositories
ln -sf $GIT_REPO_ROOT/repositories /home/git/repositories

echo "Creating gitlab-shell log file fifo pipe for logging."
# Creating a pipe so that gitlab-shell processes can write into something that does not increase in size
mkfifo /home/git/gitlab-shell/gitlab-shell.log
# Tailing the pipe as this is the process that the docker container is logging to stdout. Otherwise, no logs would be visible to stdout as they execute
# as other processes from the ssh commands from fh-scm
echo "Starting tail of gitlab-shell log file to output all logs to stdout"
(tail -f /home/git/gitlab-shell/gitlab-shell.log ; echo "gitlab-shell logging stopped unexpectedly") &

### Setup permissions
chmod -R o-rwx $GIT_REPO_ROOT
chmod 700 $GIT_REPO_ROOT

## Enable non root ssh by removing nologin

## Run sshd deamon
/usr/sbin/sshd -D -e
