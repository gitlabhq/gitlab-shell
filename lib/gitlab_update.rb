require_relative 'gitlab_init'
require_relative 'gitlab_net'
require 'json'

class GitlabUpdate
  attr_reader :config

  def initialize(repo_path, actor, ref)
    @config = GitlabConfig.new

    @repo_path = repo_path.strip
    @repo_name = repo_path
    @repo_name.gsub!(config.repos_path.to_s, "")
    @repo_name.gsub!(/\.git$/, "")
    @repo_name.gsub!(/^\//, "")

    @actor = actor
    @ref = ref
    @ref_name = ref.gsub(/\Arefs\/(tags|heads)\//, '')

    @oldrev  = ARGV[1]
    @newrev  = ARGV[2]
  end

  def exec
    # reset GL_ID env since we already
    # get value from it
    ENV['GL_ID'] = nil

    if api.allowed?('git-receive-pack', @repo_name, @actor, @ref_name, @oldrev, @newrev)
      update_redis
      exit 0
    else
      puts "GitLab: You are not allowed to access #{@ref_name}!"
      exit 1
    end
  end

  protected

  def api
    GitlabNet.new
  end

  def update_redis
    queue = "#{config.redis_namespace}:queue:post_receive"
    msg = JSON.dump({'class' => 'PostReceive', 'args' => [@repo_path, @oldrev, @newrev, @ref, @actor]})
    unless system(*config.redis_command, 'rpush', queue, msg, err: '/dev/null', out: '/dev/null')
      puts "GitLab: An unexpected error occurred (redis-cli returned #{$?.exitstatus})."
      exit 1
    end
  end
end
