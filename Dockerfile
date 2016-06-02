FROM registry.access.redhat.com/rhscl/ruby-23-rhel7

# sshd
USER root
RUN yum install -y --setopt=tsflags=nodocs openssh-server libicu-devel && \
    yum clean all && /usr/sbin/sshd-keygen

RUN mkdir /var/run/sshd

# TODO: is this needed? https://git.io/vrplM
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

# gems
RUN ["bash", "-c", "gem install --no-ri --no-rdoc bunny"]

# git user
RUN groupadd -r git &&\
     useradd -r -s /bin/bash -g git git && \
     mkdir --parent /home/git && \
     chown -R git:git /home/git

# gitlab-shell setup
COPY . /home/git/gitlab-shell
WORKDIR /home/git/gitlab-shell
RUN ["bash", "-c", "./bin/install"]

COPY authorized_keys /home/git/.ssh/authorized_keys
COPY dummy-redis-cli.sh /usr/bin/redis-cli


USER root
RUN chown -R git:git /home/git
EXPOSE 22
CMD [ "/usr/sbin/sshd", "-D" ]
