require 'json'

require_relative 'errors'
require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_access_status'
require_relative 'names_helper'
require_relative 'gitlab_metrics'
require_relative 'object_dirs_helper'

class GitlabAccess
  include NamesHelper

  def initialize(gl_repository, repo_path, key_id, changes, protocol)
    @gl_repository = gl_repository
    @repo_path = repo_path.strip
    @key_id = key_id
    @changes = changes.lines
    @protocol = protocol
  end

  def exec
    status = GitlabMetrics.measure('check-access:git-receive-pack') do
      api.check_access('git-receive-pack', @gl_repository, @repo_path, @key_id, @changes, @protocol, env: ObjectDirsHelper.all_attributes.to_json)
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

  private

  attr_reader :gl_repository, :repo_path, :key_id, :changes, :protocol

  def api
    GitlabNet.new
  end
end
