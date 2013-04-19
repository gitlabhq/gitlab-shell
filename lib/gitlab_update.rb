require_relative 'gitlab_init'
require_relative 'gitlab_net'

class GitlabUpdate
  def initialize(repo_path, key_id, refname)
    config = GitlabConfig.new

    @repo_path = repo_path.strip
    @repo_name = repo_path
    @repo_name.gsub!(config.repos_path.to_s, "")
    @repo_name.gsub!(/\.git$/, "")
    @repo_name.gsub!(/^\//, "")

    @key_id = key_id
    @refname = refname
    @branch_name = /refs\/heads\/([\w\.-]+)/.match(refname).to_a.last

    @oldrev  = ARGV[1]
    @newrev  = ARGV[2]

    @redis = config.redis
  end

  def exec
    # reset GL_ID env since we already
    # get value from it
    ENV['GL_ID'] = nil

    # If its push over ssh
    # we need to check user persmission per branch first
    if ssh?
      if api.allowed?('git-receive-pack', @repo_name, @key_id, @branch_name)
        update_redis
        exit 0
      else
        puts "GitLab: You are not allowed to access #{@branch_name}! "
        exit 1
      end
    else
      update_redis
      exit 0
    end
  end

  protected

  def api
    GitlabNet.new
  end

  def ssh?
    @key_id =~ /\Akey\-\d+\Z/
  end

  def update_redis
    if !@redis.empty? && !@redis.has_key?("socket")
      redis_command = "#{@redis['bin']} -h #{@redis['host']} -p #{@redis['port']}"
    elsif !@redis.empty? && @redis.has_key?("socket")
      redis_command = "#{@redis['bin']} -s #{@redis['socket']}"
    else
      # Default to old method of connecting to redis for users that haven't updated their configuration
      redis_command = "env -i redis-cli"
    end

    command = "#{redis_command} rpush '#{@redis['namespace']}:queue:post_receive' '{\"class\":\"PostReceive\",\"args\":[\"#{@repo_path}\",\"#{@oldrev}\",\"#{@newrev}\",\"#{@refname}\",\"#{@key_id}\"]}' > /dev/null 2>&1"
    system(command)
  end
end
