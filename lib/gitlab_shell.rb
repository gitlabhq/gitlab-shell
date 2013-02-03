require 'open3'
class GitlabShell
  attr_accessor :username, :repo_name, :git_cmd

  def initialize
    @username = ARGV.shift
    @origin_cmd = ENV['SSH_ORIGINAL_COMMAND']
  end

  def exec
    if @origin_cmd
      parse_cmd

      return system("git-upload-pack /home/gip/repositories/#{@repo_name}")
    else
      puts "Welcome #{@username}!"
    end
  end

  protected

  def parse_cmd
    args = @origin_cmd.split(' ')
    @git_cmd = args.shift
    @repo_name = args.shift
  end

  def git_cmds
    %w(git-upload-pack git-receive-pack git-upload-archive)
  end
end
