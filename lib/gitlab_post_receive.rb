require_relative 'gitlab_init'
require_relative 'gitlab_logger'
require 'json'
require 'bunny'

class GitlabPostReceive
  attr_reader :config, :repo_path, :changes

  def initialize(repo_path, actor, changes)
    @config = GitlabConfig.new
    @repo_path, @actor = repo_path.strip, actor
    @changes = changes.lines.to_a
  end

  def exec
    # reset GL_ID env since we already
    # get value from it
    ENV['GL_ID'] = nil

    update_redis
    update_rabbit if config.rabbit['enabled']
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

  def update_rabbit
    begin
      # Connect to rabbitmq server
      conn = Bunny.new(:hosts => config.rabbit['hosts'], :vhost => config.rabbit['vhost'], :user => config.rabbit['user'], :password => config.rabbit['password'])
      conn.start

      # Setup connection to queue
      channel = conn.create_channel
      queue = channel.queue(config.rabbit['queue'], :durable => true, :auto_delete => true)

      # Send message
      repo = @repo_path.gsub("#{config.repos_path}/", "")
      msg = JSON.dump({'type' => 'post_receive', 'repo' => repo, 'repo_path' => @repo_path, 'actor' => @actor, 'changes' => @changes})
      queue.publish(msg, :persistent => true, :content_type => "application/json")
      $logger.info { "Published post_receive message to rabbit repo=#{repo} actor=#{@actor} changes=#{@changes}" }
      # All done, clean up
      conn.close
    rescue Exception => e
      puts "Gitlab: An error occurred publishing post-receive message to rabbit"
      $logger.error { "Exception publishing post-receive message to rabbit #{e.message} #{e.backtrace.inspect}" }
    end
  end
end
