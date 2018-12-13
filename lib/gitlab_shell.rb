# frozen_string_literal: true

require 'shellwords'
require 'pathname'

require_relative 'gitlab_net'
require_relative 'gitlab_metrics'
require_relative 'action'

class GitlabShell # rubocop:disable Metrics/ClassLength
  class AccessDeniedError < StandardError; end
  class DisallowedCommandError < StandardError; end
  class InvalidRepositoryPathError < StandardError; end

  GIT_UPLOAD_PACK_COMMAND = 'git-upload-pack'
  GIT_RECEIVE_PACK_COMMAND = 'git-receive-pack'
  GIT_UPLOAD_ARCHIVE_COMMAND = 'git-upload-archive'
  GIT_LFS_AUTHENTICATE_COMMAND = 'git-lfs-authenticate'

  GITALY_COMMANDS = {
    GIT_UPLOAD_PACK_COMMAND => File.join(ROOT_PATH, 'bin', 'gitaly-upload-pack'),
    GIT_UPLOAD_ARCHIVE_COMMAND => File.join(ROOT_PATH, 'bin', 'gitaly-upload-archive'),
    GIT_RECEIVE_PACK_COMMAND => File.join(ROOT_PATH, 'bin', 'gitaly-receive-pack')
  }.freeze

  GIT_COMMANDS = (GITALY_COMMANDS.keys + ['git-lfs-authenticate']).freeze
  API_COMMANDS = %w(2fa_recovery_codes).freeze
  GL_PROTOCOL = 'ssh'

  attr_accessor :gl_id, :gl_repository, :repo_name, :command, :git_access, :git_protocol

  def initialize(who)
    who_sym, = GitlabNet.parse_who(who)
    if who_sym == :username
      @who = who
    else
      @gl_id = who
    end
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
    args = parse_cmd(args)

    access_status = nil

    if GIT_COMMANDS.include?(args.first)
      access_status = GitlabMetrics.measure('verify-access') { verify_access }

      @gl_repository = access_status.gl_repository
      @git_protocol = ENV['GIT_PROTOCOL']
      @gitaly = access_status.gitaly
      @username = access_status.gl_username
      @git_config_options = access_status.git_config_options
      @gl_id = access_status.gl_id if defined?(@who)
    elsif !defined?(@gl_id)
      # We're processing an API command like 2fa_recovery_codes, but
      # don't have a @gl_id yet, that means we're in the "username"
      # mode and need to materialize it, calling the "user" method
      # will do that and call the /discover method.
      user
    end

    if @command == GIT_RECEIVE_PACK_COMMAND && access_status.custom_action?
      # If the response from /api/v4/allowed is a HTTP 300, we need to perform
      # a Custom Action and therefore should return and not call process_cmd()
      #
      return process_custom_action(access_status)
    end

    process_cmd(args)

    true
  rescue GitlabNet::ApiUnreachableError
    warn "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError => ex
    $logger.warn('Access denied', command: origin_cmd, user: log_username)
    warn "GitLab: #{ex.message}"
    false
  rescue DisallowedCommandError
    $logger.warn('Denied disallowed command', command: origin_cmd, user: log_username)
    warn "GitLab: Disallowed command"
    false
  rescue InvalidRepositoryPathError
    warn "GitLab: Invalid repository path"
    false
  rescue Action::Custom::BaseError => ex
    $logger.warn('Custom action error', exception: ex.class, message: ex.message,
                                        command: origin_cmd, user: log_username)
    warn ex.message
    false
  end

  protected

  def parse_cmd(args)
    # Handle Git for Windows 2.14 using "git upload-pack" instead of git-upload-pack
    if args.length == 3 && args.first == 'git'
      @command = "git-#{args[1]}"
      args = [@command, args.last]
    else
      @command = args.first
    end

    @git_access = @command

    return args if API_COMMANDS.include?(@command)

    raise DisallowedCommandError unless GIT_COMMANDS.include?(@command)

    case @command
    when GIT_LFS_AUTHENTICATE_COMMAND
      raise DisallowedCommandError unless args.count >= 2

      @repo_name = args[1]
      case args[2]
      when 'download'
        @git_access = GIT_UPLOAD_PACK_COMMAND
      when 'upload'
        @git_access = GIT_RECEIVE_PACK_COMMAND
      else
        raise DisallowedCommandError
      end
    else
      raise DisallowedCommandError unless args.count == 2

      @repo_name = args.last
    end

    args
  end

  def verify_access
    status = api.check_access(@git_access, nil, @repo_name, @who || @gl_id, '_any', GL_PROTOCOL)

    raise AccessDeniedError, status.message unless status.allowed?

    status
  end

  def process_custom_action(access_status)
    Action::Custom.new(@gl_id, access_status.payload).execute
  end

  def process_cmd(args)
    return send("api_#{@command}") if API_COMMANDS.include?(@command)

    if @command == 'git-lfs-authenticate'
      GitlabMetrics.measure('lfs-authenticate') do
        operation = args[2]
        $logger.info('Processing LFS authentication', operation: operation, user: log_username)
        lfs_authenticate(operation)
      end
      return
    end

    # TODO: instead of building from pieces here in gitlab-shell, build the
    # entire gitaly_request in gitlab-ce and pass on as-is here.
    args = JSON.dump(
      'repository' => @gitaly['repository'],
      'gl_repository' => @gl_repository,
      'gl_id' => @gl_id,
      'gl_username' => @username,
      'git_config_options' => @git_config_options,
      'git_protocol' => @git_protocol
    )

    gitaly_address = @gitaly['address']
    executable = GITALY_COMMANDS.fetch(@command)
    gitaly_bin = File.basename(executable)
    args_string = [gitaly_bin, gitaly_address, args].join(' ')
    $logger.info('executing git command', command: args_string, user: log_username)

    exec_cmd(executable, gitaly_address: gitaly_address, token: @gitaly['token'], json_args: args)
  end

  # This method is not covered by Rspec because it ends the current Ruby process.
  def exec_cmd(executable, gitaly_address:, token:, json_args:)
    env = { 'GITALY_TOKEN' => token }

    args = [executable, gitaly_address, json_args]
    # We use 'chdir: ROOT_PATH' to let the next executable know where config.yml is.
    Kernel.exec(env, *args, unsetenv_others: true, chdir: ROOT_PATH)
  end

  def api
    GitlabNet.new
  end

  def user
    return @user if defined?(@user)

    begin
      if defined?(@who)
        @user = api.discover(@who)
        @gl_id = "user-#{@user['id']}" if @user&.key?('id')
      else
        @user = api.discover(@gl_id)
      end
    rescue GitlabNet::ApiUnreachableError
      @user = nil
    end
  end

  def username_from_discover
    return nil unless user && user['username']

    "@#{user['username']}"
  end

  def username
    @username ||= username_from_discover || 'Anonymous'
  end

  # User identifier to be used in log messages.
  def log_username
    @config.audit_usernames ? username : "user with id #{@gl_id}"
  end

  def lfs_authenticate(operation)
    lfs_access = api.lfs_authenticate(@gl_id, @repo_name, operation)

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

    resp = api.two_factor_recovery_codes(@gl_id)
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
end
