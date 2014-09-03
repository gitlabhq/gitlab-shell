require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'names_helper'
require 'json'

class GitlabAccess
  include NamesHelper

  attr_reader :config, :repo_path, :repo_name, :changes

  def initialize(repo_path, actor, changes)
    @config = GitlabConfig.new
    @repo_path, @actor = repo_path.strip, actor
    @repo_name = extract_repo_name(@repo_path.dup, config.repos_path.to_s)
    @changes = changes.lines
  end

  def exec
    if api.allowed?('git-receive-pack', @repo_name, @actor, @changes)
      exit 0
    else
      # reset GL_ID env since we stop git push here
      ENV['GL_ID'] = nil
      puts "GitLab: You are not allowed to access some of the refs!"
      exit 1
    end
  end

  protected

  def api
    GitlabNet.new
  end
end
