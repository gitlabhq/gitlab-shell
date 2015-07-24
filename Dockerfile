FROM ruby:2.1.6

# sshd
RUN apt-get update && apt-get install -y \
     openssh-server \
     libicu-dev
RUN mkdir /var/run/sshd
#RUN sed -i 's/LogLevel INFO/LogLevel VERBOSE/' /etc/ssh/sshd_config
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

# gems
ENV GEM_HOME="/usr/local/lib/ruby/gems/2.1.0"
RUN gem install --no-ri --no-rdoc \
     bunny

# git user
RUN groupadd -r git &&\
     useradd -r -s /bin/bash -g git git && \
     mkdir --parent /home/git && \
     chown -R git:git /home/git


# gitlab-shell setup
USER git
COPY . /home/git/gitlab-shell
WORKDIR /home/git/gitlab-shell
RUN ./bin/install

COPY authorized_keys /home/git/.ssh/authorized_keys
COPY dummy-redis-cli.sh /usr/bin/redis-cli


USER root
RUN chown -R git:git /home/git
EXPOSE 22
CMD [ "/usr/sbin/sshd", "-D" ]