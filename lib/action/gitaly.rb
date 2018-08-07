require_relative '../action'
require_relative '../gitlab_logger'
require_relative '../gitlab_net'

module Action
  class Gitaly < Base
    REPOSITORY_PATH_NOT_PROVIDED = "Repository path not provided. Please make sure you're using GitLab v8.10 or later.".freeze
    MIGRATED_COMMANDS = {
      'git-upload-pack' => File.join(ROOT_PATH, 'bin', 'gitaly-upload-pack'),
      'git-upload-archive' => File.join(ROOT_PATH, 'bin', 'gitaly-upload-archive'),
      'git-receive-pack' => File.join(ROOT_PATH, 'bin', 'gitaly-receive-pack')
    }.freeze

    def initialize(actor, gl_repository, gl_username, git_config_options, git_protocol, repository_path, gitaly)
      @actor = actor
      @gl_repository = gl_repository
      @gl_username = gl_username
      @git_config_options = git_config_options
      @git_protocol = git_protocol
      @repository_path = repository_path
      @gitaly = gitaly
    end

    def self.create_from_json(actor, json)
      new(actor,
          json['gl_repository'],
          json['gl_username'],
          json['git_config_options'],
          json['git_protocol'],
          json['repository_path'],
          json['gitaly'])
    end

    def execute(command, args)
      raise ArgumentError, REPOSITORY_PATH_NOT_PROVIDED unless repository_path
      raise InvalidRepositoryPathError unless valid_repository?

      $logger.info('Performing Gitaly command', user: actor.log_username)
      process(command, args)
    end

    private

    attr_reader :actor, :gl_repository, :gl_username, :git_config_options, :repository_path, :gitaly

    def git_protocol
      @git_protocol || ENV['GIT_PROTOCOL'] # TODO: tidy this up
    end

    def process(command, args)
      executable = command
      args = [repository_path]

      if MIGRATED_COMMANDS.key?(executable) && gitaly
        executable = MIGRATED_COMMANDS[executable]
        gitaly_address = gitaly['address']
        args = [gitaly_address, JSON.dump(gitaly_request)]
      end

      args_string = [File.basename(executable), *args].join(' ')
      $logger.info('executing git command', command: args_string, user: actor.log_username)

      exec_cmd(executable, *args)
    end

    def exec_cmd(*args)
      env = exec_env
      env['GITALY_TOKEN'] = gitaly['token'] if gitaly && gitaly.include?('token')

      if git_trace_available?
        env.merge!(
          'GIT_TRACE' => config.git_trace_log_file,
          'GIT_TRACE_PACKET' => config.git_trace_log_file,
          'GIT_TRACE_PERFORMANCE' => config.git_trace_log_file
        )
      end

      # We use 'chdir: ROOT_PATH' to let the next executable know where config.yml is.
      Kernel.exec(env, *args, unsetenv_others: true, chdir: ROOT_PATH)
    end

    def exec_env
      {
        'HOME' => ENV['HOME'],
        'PATH' => ENV['PATH'],
        'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'],
        'LANG' => ENV['LANG'],
        'GL_ID' => actor.identifier,
        'GL_PROTOCOL' => GitlabNet::GL_PROTOCOL,
        'GL_REPOSITORY' => gl_repository,
        'GL_USERNAME' => gl_username
      }
    end

    def gitaly_request
      # The entire gitaly_request hash should be built in gitlab-ce and passed
      # on as-is. For now we build a fake one on the spot.
      {
        'repository' => gitaly['repository'],
        'gl_repository' => gl_repository,
        'gl_id' => actor.identifier,
        'gl_username' => gl_username,
        'git_config_options' => git_config_options,
        'git_protocol' => git_protocol
      }
    end

    def valid_repository?
      File.absolute_path(repository_path) == repository_path
    end

    def git_trace_available?
      return false unless config.git_trace_log_file

      if Pathname(config.git_trace_log_file).relative?
        $logger.warn('git trace log path must be absolute, ignoring', git_trace_log_file: config.git_trace_log_file)
        return false
      end

      begin
        File.open(config.git_trace_log_file, 'a') { nil }
        return true
      rescue => ex
        $logger.warn('Failed to open git trace log file', git_trace_log_file: config.git_trace_log_file, error: ex.to_s)
        return false
      end
    end
  end
end
