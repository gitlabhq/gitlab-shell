require 'shellwords'

require_relative 'gitlab_net'

class GitlabShell
  class DisallowedCommandError < StandardError; end

  attr_accessor :key_id, :repo_name, :git_cmd, :repos_path, :repo_name

  def initialize
    @key_id = /key-[0-9]+/.match(ARGV.join).to_s
    @origin_cmd = ENV['SSH_ORIGINAL_COMMAND']
    @config = GitlabConfig.new
    @repos_path = @config.repos_path
  end

  def exec
    if @origin_cmd
      parse_cmd

      if git_cmds.include?(@git_cmd)
        ENV['GL_ID'] = @key_id

        access = api.check_access(@git_cmd, @repo_name, @key_id, '_any')

        if access.allowed?
          process_cmd
        else
          message = "gitlab-shell: Access denied for git command <#{@origin_cmd}> by #{log_username}."
          $logger.warn message
          puts access.message
        end
      else
        raise DisallowedCommandError
      end
    else
      puts "Welcome to GitLab, #{username}!"
    end
  rescue GitlabNet::ApiUnreachableError => ex
    puts "Failed to authorize your Git request: internal API unreachable"
  rescue DisallowedCommandError => ex
    message = "gitlab-shell: Attempt to execute disallowed command <#{@origin_cmd}> by #{log_username}."
    $logger.warn message
    puts 'Disallowed command'
  end

  protected

  def parse_cmd
    args = Shellwords.shellwords(@origin_cmd)
    @git_cmd = args.first

    if @git_cmd == 'git-annex-shell' && @config.git_annex_enabled?
      @repo_name = escape_path(args[2].sub(/\A\/~\//, ''))

      # Make sure repository has git-annex enabled
      init_git_annex(@repo_name)
    else
      raise DisallowedCommandError unless args.count == 2
      @repo_name = escape_path(args.last)
    end
  end

  def git_cmds
    %w(git-upload-pack git-receive-pack git-upload-archive git-annex-shell)
  end

  def process_cmd
    repo_full_path = File.join(repos_path, repo_name)

    if @git_cmd == 'git-annex-shell' && @config.git_annex_enabled?
      args = Shellwords.shellwords(@origin_cmd)
      parsed_args =
        args.map do |arg|
          # Convert /~/group/project.git to group/project.git
          # to make git annex path compatible with gitlab-shell
          if arg =~ /\A\/~\/.*\.git\Z/
            repo_full_path
          else
            arg
          end
        end

      $logger.info "gitlab-shell: executing git-annex command <#{parsed_args.join(' ')}> for #{log_username}."
      exec_cmd(*parsed_args)
    else
      $logger.info "gitlab-shell: executing git command <#{@git_cmd} #{repo_full_path}> for #{log_username}."
      exec_cmd(@git_cmd, repo_full_path)
    end
  end

  # This method is not covered by Rspec because it ends the current Ruby process.
  def exec_cmd(*args)
    Kernel::exec({ 'PATH' => ENV['PATH'], 'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'], 'GL_ID' => ENV['GL_ID'] }, *args, unsetenv_others: true)
  end

  def api
    GitlabNet.new
  end

  def user
    return @user if defined?(@user)

    begin
      @user = api.discover(@key_id)
    rescue GitlabNet::ApiUnreachableError
      @user = nil
    end
  end

  def username
    user && user['name'] || 'Anonymous'
  end

  # User identifier to be used in log messages.
  def log_username
    @config.audit_usernames ? username : "user with key #{@key_id}"
  end

  def escape_path(path)
    full_repo_path = File.join(repos_path, path)

    if File.absolute_path(full_repo_path) == full_repo_path
      path
    else
      abort "Wrong repository path"
    end
  end

  def init_git_annex(path)
    full_repo_path = File.join(repos_path, path)

    unless File.exists?(File.join(full_repo_path, 'annex'))
      cmd = %W(git --git-dir=#{full_repo_path} annex init "GitLab")
      system(*cmd, err: '/dev/null', out: '/dev/null')
      $logger.info "Enable git-annex for repository: #{path}."
    end
  end
end
