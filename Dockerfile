FROM registry.access.redhat.com/rhscl/ruby-23-rhel7

# sshd
USER root
RUN ["bash", "-c", "yum install -y --setopt=tsflags=nodocs openssh-server libicu-devel && \
     yum clean all && \
     sshd-keygen && \
     mkdir /var/run/sshd && \
     /usr/sbin/groupadd -g 48 git && \
     useradd -r -m -s /bin/bash -u 48 -g 48 git"]

# gitlab-shell setup
COPY . /home/git/gitlab-shell
WORKDIR /home/git/gitlab-shell

RUN ["bash", "-c", "bundle"]

RUN mkdir /home/git/gitlab-config && \
    ## Setup default config placeholder
    cp config.yml.example ../gitlab-config/config.yml && \
    ln -s /home/git/gitlab-config/config.yml && \
    # PAM workarounds for docker and public key auth
    sed -i \
          # Disable processing of user uid. See: https://gitlab.com/gitlab-org/gitlab-ce/issues/3027
          -e "s|session\s*required\s*pam_loginuid.so|session optional pam_loginuid.so|g" \
          # Allow non root users to login: http://man7.org/linux/man-pages/man8/pam_nologin.8.html
          -e "s|account\s*required\s*pam_nologin.so|#account optional pam_nologin.so|g" \
          /etc/pam.d/sshd && \
    # Security recommendations for sshd
    sed -i \
          -e "s|^[#]*GSSAPIAuthentication yes|GSSAPIAuthentication no|" \
          -e "s|^[#]*ChallengeResponseAuthentication no|ChallengeResponseAuthentication no|" \
          -e "s|^[#]*PasswordAuthentication yes|PasswordAuthentication no|" \
          -e "s|^[#]*StrictModes yes|StrictModes no|" \
          /etc/ssh/sshd_config && \
    echo -e "UseDNS no \nAuthenticationMethods publickey" >> /etc/ssh/sshd_config && \
    chmod -Rf +x /home/git/gitlab-shell/bin

EXPOSE 22
CMD ["bin/start.sh"]
