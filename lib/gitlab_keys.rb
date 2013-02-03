require 'open3'
require 'yaml'

class GitlabKeys
  attr_accessor :auth_file, :key, :username

  def initialize
    @command = ARGV.shift
    @username = ARGV.shift
    @key = ARGV.shift

    config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
    @auth_file = config['auth_file']
  end

  def exec
    case @command
    when 'add-key'; add_key
    when 'rm-key';  rm_key
    when 'rm-user'; rm_user
    else
      puts 'not allowed'
    end
  end

  protected

  def add_key
    cmd = "command=\"#{ROOT_PATH}/bin/gitlab-shell #{@username}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty #{@key}"
    cmd = "echo \"#{cmd}\" >> #{auth_file}"
    system(cmd)
  end

  def rm_key
    cmd = "sed '/#{@key}/d' #{auth_file}"
    system(cmd)
  end

  def rm_user
    cmd = "sed -i '/#{@username}/d' #{auth_file}"
    puts cmd
    system(cmd)
  end
end
