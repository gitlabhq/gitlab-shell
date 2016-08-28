require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_access_status'
require_relative 'names_helper'
require 'json'

class GitlabAccess
  class AccessDeniedError < StandardError; end
  class MemoryLimitReachedError < StandardError; end

  include NamesHelper

  attr_reader :config, :repo_path, :repo_name, :changes, :protocol

  def initialize(repo_path, actor, changes, protocol)
    @config = GitlabConfig.new
    @repo_path = repo_path.strip
    @actor = actor
    @repo_name = extract_repo_name(@repo_path.dup)
    @changes = changes.lines
    @protocol = protocol
  end

  def exec
    status = api.check_access('git-receive-pack', @repo_name, @actor, @changes, @protocol)

    raise AccessDeniedError, status.message unless status.allowed?
    raise MemoryLimitReachedError, status.memory_message if status.memory_limit?

    true
  rescue GitlabNet::ApiUnreachableError
    $stderr.puts "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError => ex
    $stderr.puts "GitLab: #{ex.message}"
    false
  rescue MemoryLimitReachedError => ex
    $stderr.puts "Gitlab: Project has reached the memory limit by: #{ex.message}MB please free some memory in Gitlab's UI to be able to push."
    false
  end

  protected

  def api
    GitlabNet.new
  end
end
