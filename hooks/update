#!/usr/bin/env ruby

# This file was placed here by GitLab. It makes sure that your pushed commits
# will be processed properly.

ref_name = ARGV[0]
old_value = ARGV[1]
new_value = ARGV[2]
repo_path = Dir.pwd

require_relative '../lib/gitlab_custom_hook'

if GitlabCustomHook.new.update(ref_name, old_value, new_value, repo_path)
  exit 0
else
  exit 1
end
