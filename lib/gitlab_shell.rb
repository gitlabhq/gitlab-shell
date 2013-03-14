require 'open3'

require_relative 'gitlab_net'

class GitlabShell
  attr_accessor :key_id, :repo_name, :git_cmd, :repos_path, :repo_name

  def initialize
    @key_id = ARGV.shift
    @origin_cmd = ENV['SSH_ORIGINAL_COMMAND']
    @repos_path = GitlabConfig.new.repos_path
  end

  def exec
    if @origin_cmd
      parse_cmd

      if git_cmds.include?(@git_cmd)
        ENV['GL_ID'] = @key_id

        if validate_access
          process_cmd
        end
      else
        if admin?
          process_admin_cmd
        else
          puts 'Not allowed command'
        end
      end
    else
      if admin?
        # Execute the shell.
        process_admin_cmd
      else
        user = api.discover(@key_id)
        puts "Welcome to GitLab, #{user['name']}!"
      end
    end
  end

  protected

  # Wrapper around Kernel::exec
  def exec_cmd args
    Kernel::exec args
  end

  def parse_cmd
    args = @origin_cmd.split(' ')
    @git_cmd = args.shift
    @repo_name = args.shift
  end

  def git_cmds
    %w(git-upload-pack git-receive-pack git-upload-archive)
  end

  def process_cmd
    ENV.delete 'SSH_TTY'       # Disable TTY
    ENV.delete 'SSH_AUTH_SOCK' # Disable SSH forwarding
    repo_full_path = File.join(repos_path, repo_name)
    exec_cmd("#{@git_cmd} #{repo_full_path}")
  end

  def process_admin_cmd
    args = @origin_cmd ? @origin_cmd : [ ENV["SHELL"], "-l" ]
    exec_cmd(args)
  end

  def admin?
    api.admin?(@key_id)
  end

  def validate_access
    api.allowed?(@git_cmd, @repo_name, @key_id, '_any')
  end

  def api
    GitlabNet.new
  end
end
