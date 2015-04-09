require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_access_status'
require_relative 'names_helper'
require 'json'

class GitlabAccess
  class AccessDeniedError < StandardError; end

  include NamesHelper

  attr_reader :config, :repo_path, :repo_name, :changes

  def initialize(repo_path, actor, changes)
    @config = GitlabConfig.new
    @repo_path = repo_path.strip
    @actor = actor
    @repo_name = extract_repo_name(@repo_path.dup, config.repos_path.to_s)
    @changes = changes.lines
  end

  def exec
    status = api.check_access('git-receive-pack', @repo_name, @actor, @changes)

    raise AccessDeniedError, status.message unless status.allowed?

    true
  rescue GitlabNet::ApiUnreachableError
    $stderr.puts "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError => ex
    $stderr.puts "GitLab: #{ex.message}"
    false
  end

  protected

  def api
    GitlabNet.new
  end
end
