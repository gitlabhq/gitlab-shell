#!/usr/bin/env ruby

# This file was placed here by GitLab. It makes sure that your pushed commits
# will be processed properly.
# You can add your own hooks to this file, but be careful when updating gitlab-shell!

refname = ARGV[0]
key_id  = ENV['GL_ID']
repo_path = `pwd`

require_relative '../lib/gitlab_update'

GitlabUpdate.new(repo_path, key_id, refname).exec
