#!/usr/bin/env ruby

unless ENV['SSH_CONNECTION']
  puts "Only ssh allowed"
  exit
end

require_relative '../lib/gitlab_init'

#
#
# GitLab shell, invoked from ~/.ssh/authorized_keys
#
#
require File.join(ROOT_PATH, 'lib', 'gitlab_shell')
GitlabShell.new.exec

exit
