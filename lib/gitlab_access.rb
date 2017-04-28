require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_access_status'
require_relative 'names_helper'
require_relative 'gitlab_metrics'
require 'json'

class GitlabAccess
  class AccessDeniedError < StandardError; end

  include NamesHelper

  attr_reader :config, :gl_repository, :repo_path, :changes, :protocol

  def initialize(gl_repository, repo_path, actor, changes, protocol)
    @config = GitlabConfig.new
    @gl_repository = gl_repository
    @repo_path = repo_path.strip
    @actor = actor
    @changes = changes.lines
    @protocol = protocol
  end

  def exec
    status = GitlabMetrics.measure('check-access:git-receive-pack') do
      env = {
        "GIT_ALTERNATE_OBJECT_DIRECTORIES" => ENV["GIT_ALTERNATE_OBJECT_DIRECTORIES"],
        "GIT_OBJECT_DIRECTORY" => ENV["GIT_OBJECT_DIRECTORY"]
      }

      api.check_access('git-receive-pack', @gl_repository, @repo_path, @actor, @changes, @protocol, env: env.to_json)
    end

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
