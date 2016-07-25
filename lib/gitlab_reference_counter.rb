require_relative 'gitlab_init'
require_relative 'gitlab_net'

class GitlabReferenceCounter
  REFERENCE_EXPIRE_TIME = 600

  attr_reader :path, :key

  def initialize(path)
    @path = path
    @key = "git-receive-pack-reference-counter:#{path}"
  end

  def value
    (redis_client.get(key) || 0).to_i
  end

  def increase
    redis_cmd do
      redis_client.incr(key)
      redis_client.expire(key, REFERENCE_EXPIRE_TIME)
    end
  end

  def decrease
    redis_cmd do
      current_value = redis_client.decr(key)
      if current_value < 0
        $logger.warn "Reference counter for #{path} decreased when its value was less than 1. Reseting the counter."
        redis_client.del(key)
      end
    end
  end

  private

  def redis_client
    @redis_client ||= GitlabNet.new.redis_client
  end

  def redis_cmd
    begin
      yield
      true
    rescue => e
      message = "GitLab: An unexpected error occurred in writing to Redis: #{e}"
      $stderr.puts message
      $logger.error message
      false
    end
  end
end
