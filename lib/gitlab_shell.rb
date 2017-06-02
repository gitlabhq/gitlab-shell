require 'shellwords'
require 'pathname'

require_relative 'gitlab_net'
require_relative 'gitlab_metrics'

class GitlabShell
  class AccessDeniedError < StandardError; end
  class DisallowedCommandError < StandardError; end
  class InvalidRepositoryPathError < StandardError; end

  GIT_COMMANDS = %w(git-upload-pack git-receive-pack git-upload-archive git-lfs-authenticate).freeze
  API_COMMANDS = %w(2fa_recovery_codes)
  GL_PROTOCOL = 'ssh'.freeze

  attr_accessor :key_id, :repo_name, :command, :git_access
  attr_reader :repo_path

  def initialize(key_id)
    @key_id = key_id
    @config = GitlabConfig.new
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

    if GIT_COMMANDS.include?(args.first)
      GitlabMetrics.measure('verify-access') { verify_access }
    end

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
    @command = args.first
    @git_access = @command

    return if API_COMMANDS.include?(@command)

    raise DisallowedCommandError unless GIT_COMMANDS.include?(@command)

    case @command
    when 'git-lfs-authenticate'
      raise DisallowedCommandError unless args.count >= 2
      @repo_name = args[1]
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
      @repo_name = args.last
    end
  end

  def verify_access
    status = api.check_access(@git_access, @repo_name, @key_id, '_any', GL_PROTOCOL)

    raise AccessDeniedError, status.message unless status.allowed?

    self.repo_path = status.repository_path
  end

  def process_cmd(args)
    return self.send("api_#{@command}") if API_COMMANDS.include?(@command)

    if @command == 'git-lfs-authenticate'
      GitlabMetrics.measure('lfs-authenticate') do
        $logger.info "gitlab-shell: Processing LFS authentication for #{log_username}."
        lfs_authenticate
      end
    else
      $logger.info "gitlab-shell: executing git command <#{@command} #{repo_path}> for #{log_username}."
      exec_cmd(@command, repo_path)
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
      'GL_ID' => @key_id,
      'GL_USERNAME' => username,
      'GL_LOGIN' => login,
      'GL_PROTOCOL' => GL_PROTOCOL
    }

    if git_trace_available?
      env.merge!({
        'GIT_TRACE' => @config.git_trace_log_file,
        'GIT_TRACE_PACKET' => @config.git_trace_log_file,
        'GIT_TRACE_PERFORMANCE' => @config.git_trace_log_file,
      })
    end

    Kernel::exec(env, *args, unsetenv_others: true)
  end

  def api
    GitlabNet.new
  end

  def user
    return @user if defined?(@user)

    if File.file?(@config.secret_file)
      begin
        @user = api.discover(@key_id)
      rescue GitlabNet::ApiUnreachableError
        @user = nil
      end
    else
      @user = nil
    end
  end

  def username
    user && user['name'] || 'Anonymous'
  end

  def login
    user && user['username'] || 'anonymous'
  end

  # User identifier to be used in log messages.
  def log_username
    @config.audit_usernames ? username : "user with key #{@key_id}"
  end

  def lfs_authenticate
    lfs_access = api.lfs_authenticate(@key_id, @repo_name)

    return unless lfs_access

    puts lfs_access.authentication_payload
  end

  private

  def continue?(question)
    puts "#{question} (yes/no)"
    STDOUT.flush # Make sure the question gets output before we wait for input
    continue = STDIN.gets.chomp
    puts '' # Add a buffer in the output
    continue == 'yes'
  end

  def api_2fa_recovery_codes
    continue = continue?(
      "Are you sure you want to generate new two-factor recovery codes?\n" \
      "Any existing recovery codes you saved will be invalidated."
    )

    unless continue
      puts 'New recovery codes have *not* been generated. Existing codes will remain valid.'
      return
    end

    resp = api.two_factor_recovery_codes(key_id)
    if resp['success']
      codes = resp['recovery_codes'].join("\n")
      puts "Your two-factor authentication recovery codes are:\n\n" \
           "#{codes}\n\n" \
           "During sign in, use one of the codes above when prompted for\n" \
           "your two-factor code. Then, visit your Profile Settings and add\n" \
           "a new device so you do not lose access to your account again."
    else
      puts "An error occurred while trying to generate new recovery codes.\n" \
           "#{resp['message']}"
    end
  end

  def git_trace_available?
    return false unless @config.git_trace_log_file

    if Pathname(@config.git_trace_log_file).relative?
      $logger.warn "gitlab-shell: is configured to trace git commands with #{@config.git_trace_log_file.inspect} but an absolute path needs to be provided"
      return false
    end

    begin
      File.open(@config.git_trace_log_file, 'a') { nil }
      return true
    rescue => ex
      $logger.warn "gitlab-shell: is configured to trace git commands with #{@config.git_trace_log_file.inspect} but it's not possible to write in that path #{ex.message}"
      return false
    end
  end

  def repo_path=(repo_path)
    raise ArgumentError, "Repository path not provided. Please make sure you're using GitLab v8.10 or later." unless repo_path
    raise InvalidRepositoryPathError if File.absolute_path(repo_path) != repo_path

    @repo_path = repo_path
  end
end
