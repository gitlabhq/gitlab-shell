require 'yaml'

class GitlabConfig
  attr_reader :config

  def initialize
    @config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
  end

  def repos_path
    @config['repos_path'] ||= "/home/git/repositories"
  end

  def auth_file
    @config['auth_file'] ||= "/home/git/.ssh/authorized_keys"
  end

  def gitlab_url
    @config['gitlab_url'] ||= "http://localhost/"
  end

  def http_settings
    @config['http_settings'] ||= {}
  end

  def redis
    @config['redis'] ||= {}
  end

  def redis_namespace
    redis['namespace'] || 'resque:gitlab'
  end

  def log_file
    @config['log_file'] ||= File.join(ROOT_PATH, 'gitlab-shell.log')
  end

  def log_level
    @config['log_level'] ||= 'INFO'
  end

  def audit_usernames
    @config['audit_usernames'] ||= false
  end

  # Build redis command to write update event in gitlab queue
  def redis_command
    if redis.empty?
      # Default to old method of connecting to redis
      # for users that haven't updated their configuration
      %W(env -i redis-cli)
    else
      if redis.has_key?("socket")
        %W(#{redis['bin']} -s #{redis['socket']})
      else
        if redis.has_key?("pass")
          %W(#{redis['bin']} -h #{redis['host']} -p #{redis['port']} -a #{redis['pass']})
        else
          %W(#{redis['bin']} -h #{redis['host']} -p #{redis['port']})
        end
      end
    end
  end
end
