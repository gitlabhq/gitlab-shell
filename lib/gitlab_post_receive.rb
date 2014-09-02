require_relative 'gitlab_init'
require 'json'

class GitlabPostReceive
  attr_reader :config, :repo_path, :changes

  def initialize(repo_path, actor, changes)
    @config = GitlabConfig.new
    @repo_path, @actor = repo_path.strip, actor
    @changes = changes.lines
  end

  def exec
    # reset GL_ID env since we already
    # get value from it
    ENV['GL_ID'] = nil

    update_redis
  end

  protected

  def update_redis
    queue = "#{config.redis_namespace}:queue:post_receive"
    msg = JSON.dump({'class' => 'PostReceive', 'args' => [@repo_path, @actor, @changes]})
    unless system(*config.redis_command, 'rpush', queue, msg, err: '/dev/null', out: '/dev/null')
      puts "GitLab: An unexpected error occurred (redis-cli returned #{$?.exitstatus})."
      exit 1
    end
  end
end
