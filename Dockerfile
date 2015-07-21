FROM ruby:2.1.6

# sshd
RUN apt-get update && apt-get install -y \
    openssh-server

# git user
RUN groupadd -f -g git git && \
    useradd -u git -g git git && \
    mkdir --parent /home/git && \
    chown -R git:git /home/git

# gitlab-shell setup
USER git
WORKDIR /home/git
RUN ./bin/install

COPY authorized_keys /home/git/.ssh/authorized_keys
COPY dummy-redis-cli.sh /usr/local/bin/redis-cli

EXPOSE 22

CMD [ "tail", "-f", "/home/git/gitlab-shell/gitlab-shell.log"]