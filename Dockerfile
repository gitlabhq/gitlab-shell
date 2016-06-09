FROM registry.access.redhat.com/rhscl/ruby-23-rhel7

# sshd
USER root
RUN ["bash", "-c", "yum install -y --setopt=tsflags=nodocs openssh-server libicu-devel && \
     yum clean all && \
     sshd-keygen && \
     mkdir /var/run/sshd && \
     gem install --no-ri --no-rdoc bunny && \
     groupadd -r git && \
     useradd -r -s /bin/bash -g git git && \
     mkdir --parent /home/git"]

# https://gitlab.com/gitlab-org/gitlab-ce/issues/3027
# https://docs.docker.com/engine/examples/running_ssh_service/
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd

# gitlab-shell setup
COPY . /home/git/gitlab-shell
WORKDIR /home/git/gitlab-shell

COPY authorized_keys /home/git/.ssh/authorized_keys

RUN ["bash", "-c", "mkdir ../gitlab-config && \
                     cp config.yml.example ../gitlab-config/config.yml && \
                     ln -s ../gitlab-config/config.yml && \
                     ./bin/install && \
                     chown -R git:git /home/git"]

EXPOSE 22
CMD [ "/usr/sbin/sshd", "-D" ]
