require 'open3'
require_relative 'gitlab_config'

class GitlabKeys
  attr_accessor :auth_file, :key, :username

  def initialize
    @command = ARGV.shift
    @username = ARGV.shift
    @key = ARGV.shift
    @auth_file = GitlabConfig.new.auth_file
  end

  def exec
    case @command
    when 'add-key'; add_key
    when 'rm-key';  rm_key
    else
      puts 'not allowed'
    end
  end

  protected

  def add_key
    cmd = "command=\"#{ROOT_PATH}/bin/gitlab-shell #{@username}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty #{@key}"
    cmd = "echo \'#{cmd}\' >> #{auth_file}"
    system(cmd)
  end

  def rm_key
    cmd = "sed '/#{@key}/d' #{auth_file}"
    system(cmd)
  end
end
