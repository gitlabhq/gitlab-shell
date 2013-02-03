require 'open3'
require 'yaml'

class GitlabShell
  attr_accessor :username, :repo_name, :git_cmd, :repos_path

  def initialize
    @username = ARGV.shift
    @origin_cmd = ENV['SSH_ORIGINAL_COMMAND']
    @config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
    @repos_path = @config['repos_path']
  end

  def exec
    if @origin_cmd
      parse_cmd

      if git_cmds.include?(@git_cmd)
        process_cmd
      else
        puts 'Not allowed command'
      end
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

  def process_cmd
    repo_full_path = File.join(repos_path, repo_name)
    system("#{@git_cmd} #{repo_full_path}")
  end
end
