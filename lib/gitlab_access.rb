require 'json'

require_relative 'errors'
require_relative 'actor'
require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'names_helper'
require_relative 'gitlab_metrics'
require_relative 'object_dirs_helper'

class GitlabAccess
  include NamesHelper

  def initialize(gl_repository, repo_path, gl_id, changes, protocol)
    @gl_repository = gl_repository
    @repo_path = repo_path.strip
    @gl_id = gl_id
    @changes = changes.lines
    @protocol = protocol
  end

  def exec
    GitlabMetrics.measure('check-access:git-receive-pack') do
      api.check_access('git-receive-pack', gl_repository, repo_path, actor, changes, protocol, env: ObjectDirsHelper.all_attributes.to_json)
    end
    true
  rescue GitlabNet::ApiUnreachableError
    $stderr.puts "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError => ex
    $stderr.puts "GitLab: #{ex.message}"
    false
  end

  private

  attr_reader :gl_repository, :repo_path, :gl_id, :changes, :protocol

  def api
    @api ||= GitlabNet.new
  end

  def config
    @config ||= GitlabConfig.new
  end

  def actor
    @actor ||= Actor.new_from(gl_id, audit_usernames: config.audit_usernames)
  end
end
