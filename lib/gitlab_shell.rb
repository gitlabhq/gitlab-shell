require 'shellwords'

require_relative 'gitlab_net'

class GitlabShell
  class AccessDeniedError < StandardError; end
  class DisallowedCommandError < StandardError; end
  class InvalidRepositoryPathError < StandardError; end

  GIT_COMMANDS = %w(git-upload-pack git-receive-pack git-upload-archive git-annex-shell git-lfs-authenticate).freeze

  attr_accessor :key_id, :repo_name, :git_cmd, :repos_path, :repo_name

  def initialize(key_id)
    @key_id = key_id
    @config = GitlabConfig.new
    @repos_path = @config.repos_path
  end

  # The origin_cmd variable contains UNTRUSTED input. If the user ran
  # ssh git@gitlab.example.com 'evil command', then origin_cmd contains
  # 'evil command'.
  def exec(origin_cmd)
    unless origin_cmd
      puts "Welcome to GitLab, #{username}!"
      return true
    end

    args = Shellwords.shellwords(origin_cmd)
    parse_cmd(args)

    verify_access

    process_cmd(args)

    true
  rescue GitlabNet::ApiUnreachableError => ex
    $stderr.puts "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError => ex
    message = "gitlab-shell: Access denied for git command <#{origin_cmd}> by #{log_username}."
    $logger.warn message

    $stderr.puts "GitLab: #{ex.message}"
    false
  rescue DisallowedCommandError => ex
    message = "gitlab-shell: Attempt to execute disallowed command <#{origin_cmd}> by #{log_username}."
    $logger.warn message

    $stderr.puts "GitLab: Disallowed command"
    false
  rescue InvalidRepositoryPathError => ex
    $stderr.puts "GitLab: Invalid repository path"
    false
  end

  protected

  def parse_cmd(args)
    @git_cmd = args.first
    @git_access = @git_cmd

    raise DisallowedCommandError unless GIT_COMMANDS.include?(@git_cmd)

    case @git_cmd
    when 'git-annex-shell'
      raise DisallowedCommandError unless @config.git_annex_enabled?

      @repo_name = escape_path(args[2].sub(/\A\/~\//, ''))

      # Make sure repository has git-annex enabled
      init_git_annex(@repo_name) unless gcryptsetup?(args)
    when 'git-lfs-authenticate'
      raise DisallowedCommandError unless args.count >= 2
      @repo_name = escape_path(args[1])
      case args[2]
      when 'download'
        @git_access = 'git-upload-pack'
      when 'upload'
        @git_access = 'git-receive-pack'
      else
        raise DisallowedCommandError
      end
    else
      raise DisallowedCommandError unless args.count == 2
      @repo_name = escape_path(args.last)
    end
  end

  def verify_access
    status = api.check_access(@git_access, @repo_name, @key_id, '_any')

    raise AccessDeniedError, status.message unless status.allowed?
  end

  def process_cmd(args)
    repo_full_path = File.join(repos_path, repo_name)

    if @git_cmd == 'git-annex-shell'
      raise DisallowedCommandError unless @config.git_annex_enabled?

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
    # If you want to call a command without arguments, use
    # exec_cmd(['my_command', 'my_command']) . Otherwise use
    # exec_cmd('my_command', 'my_argument', ...).
    if args.count == 1 && !args.first.is_a?(Array)
      raise DisallowedCommandError
    end

    env = {
      'HOME' => ENV['HOME'],
      'PATH' => ENV['PATH'],
      'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'],
      'LANG' => ENV['LANG'],
      'GL_ID' => @key_id
    }

    if @config.git_annex_enabled?
      env.merge!({ 'GIT_ANNEX_SHELL_LIMITED' => '1' })
    end

    Kernel::exec(env, *args, unsetenv_others: true)
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
      raise InvalidRepositoryPathError
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

  def gcryptsetup?(args)
    non_dashed = args.reject { |a| a.start_with?('-') }
    non_dashed[0, 2] == %w{git-annex-shell gcryptsetup}
  end
end
