require 'open3'

class GitlabKeys
  attr_accessor :auth_file, :key

  def initialize
    @command = ARGV.shift
    @key_id = ARGV.shift
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
    cmd = "command=\"#{ROOT_PATH}/bin/gitlab-shell #{@key_id}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty #{@key}"
    cmd = "echo \'#{cmd}\' >> #{auth_file}"
    system(cmd)
  end

  def rm_key
    cmd = "sed -i '/shell #{@key_id}/d' #{auth_file}"
    system(cmd)
  end
end
