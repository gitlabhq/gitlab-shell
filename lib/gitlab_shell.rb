require 'shellwords'
require 'pathname'

require_relative 'gitlab_net'
require_relative 'gitlab_metrics'
require_relative 'actor'

class GitlabShell
  API_2FA_RECOVERY_CODES_COMMAND = '2fa_recovery_codes'.freeze

  GIT_UPLOAD_PACK_COMMAND = 'git-upload-pack'.freeze
  GIT_RECEIVE_PACK_COMMAND = 'git-receive-pack'.freeze
  GIT_UPLOAD_ARCHIVE_COMMAND = 'git-upload-archive'.freeze
  GIT_LFS_AUTHENTICATE_COMMAND = 'git-lfs-authenticate'.freeze

  GIT_COMMANDS = [GIT_UPLOAD_PACK_COMMAND, GIT_RECEIVE_PACK_COMMAND,
                  GIT_UPLOAD_ARCHIVE_COMMAND, GIT_LFS_AUTHENTICATE_COMMAND].freeze

  Struct.new('ParsedCommand', :command, :git_access_command, :repo_name, :args)

  def initialize(who)
    @config = GitlabConfig.new
    @actor = Actor.new_from(who, audit_usernames: @config.audit_usernames)
  end

  # The origin_cmd variable contains UNTRUSTED input. If the user ran
  # ssh git@gitlab.example.com 'evil command', then origin_cmd contains
  # 'evil command'.
  def exec(origin_cmd)
    if !origin_cmd || origin_cmd.empty?
      puts "Welcome to GitLab, #{actor.username}!"
      return true
    end

    parsed_command = parse_cmd(origin_cmd)
    action = determine_action(parsed_command)
    action.execute(parsed_command.command, parsed_command.args)
  rescue GitlabNet::ApiUnreachableError
    $stderr.puts "GitLab: Failed to authorize your Git request: internal API unreachable"
    false
  rescue AccessDeniedError, UnknownError => ex
    $logger.warn('Access denied', command: origin_cmd, user: actor.log_username)
    $stderr.puts "GitLab: #{ex.message}"
    false
  rescue DisallowedCommandError
    $logger.warn('Denied disallowed command', command: origin_cmd, user: actor.log_username)
    $stderr.puts 'GitLab: Disallowed command'
    false
  rescue InvalidRepositoryPathError
    $stderr.puts 'GitLab: Invalid repository path'
    false
  end

  private

  attr_reader :config, :actor

  def parse_cmd(cmd)
    args = Shellwords.shellwords(cmd)

    # Handle Git for Windows 2.14 using "git upload-pack" instead of git-upload-pack
    if args.length == 3 && args.first == 'git'
      command = "git-#{args[1]}"
      args = [command, args.last]
    else
      command = args.first
    end

    git_access_command = command

    if command == API_2FA_RECOVERY_CODES_COMMAND
      return Struct::ParsedCommand.new(command, git_access_command, nil, args)
    end

    raise DisallowedCommandError unless GIT_COMMANDS.include?(command)

    case command
    when 'git-lfs-authenticate'
      raise DisallowedCommandError unless args.count >= 2
      repo_name = args[1]
      git_access_command = case args[2]
                           when 'download'
                             GIT_UPLOAD_PACK_COMMAND
                           when 'upload'
                             GIT_RECEIVE_PACK_COMMAND
                           else
                             raise DisallowedCommandError
                           end
    else
      raise DisallowedCommandError unless args.count == 2
      repo_name = args.last
    end

    Struct::ParsedCommand.new(command, git_access_command, repo_name, args)
  end

  def determine_action(parsed_command)
    return Action::API2FARecovery.new(actor) if parsed_command.command == API_2FA_RECOVERY_CODES_COMMAND

    GitlabMetrics.measure('verify-access') do
      # GitlabNet#check_access will raise exception in the event of a problem
      initial_action = api.check_access(
        parsed_command.git_access_command,
        nil,
        parsed_command.repo_name,
        actor,
        '_any'
      )

      case parsed_command.command
      when GIT_LFS_AUTHENTICATE_COMMAND
        Action::GitLFSAuthenticate.new(actor, parsed_command.repo_name)
      else
        initial_action
      end
    end
  end

  def api
    @api ||= GitlabNet.new
  end
end
