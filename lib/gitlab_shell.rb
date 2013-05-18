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
        else
          message = "gitlab-shell: Access denied for git command <#{@origin_cmd}>"
          message << " by user with key #{@key_id}."
          $logger.warn message
        end
      else
        message = "gitlab-shell: Attempt to execute disallowed command "
        message << "<#{@origin_cmd}> by user with key #{@key_id}."
        $logger.warn message
        puts 'Not allowed command'
      end
    else
      user = api.discover(@key_id)
      puts "Welcome to GitLab, #{user && user['name'] || 'Anonymous'}!"
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
    cmd = "#{@git_cmd} #{repo_full_path}"
    $logger.info "gitlab-shell: executing git command <#{cmd}> for user with key #{@key_id}."
    exec_cmd(cmd)
  end

  def validate_access
    api.allowed?(@git_cmd, @repo_name, @key_id, '_any')
  end

  def exec_cmd args
    Kernel::exec args
  end

  def api
    GitlabNet.new
  end
end
