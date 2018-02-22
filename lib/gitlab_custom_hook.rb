require 'open3'
require_relative 'gitlab_init'
require_relative 'gitlab_metrics'

class GitlabCustomHook
  attr_reader :vars, :config

  def initialize(repo_path, key_id)
    @repo_path = repo_path
    @vars = { 'GL_ID' => key_id }
    @config = GitlabConfig.new
  end

  def pre_receive(changes)
    GitlabMetrics.measure("pre-receive-hook") do
      find_hooks('pre-receive').all? do |hook|
        call_receive_hook(hook, changes)
      end
    end
  end

  def post_receive(changes)
    GitlabMetrics.measure("post-receive-hook") do
      find_hooks('post-receive').all? do |hook|
        call_receive_hook(hook, changes)
      end
    end
  end

  def update(ref_name, old_value, new_value)
    GitlabMetrics.measure("update-hook") do
      find_hooks('update').all? do |hook|
        system(vars, hook, ref_name, old_value, new_value)
      end
    end
  end

  private

  def call_receive_hook(hook, changes)
    # Prepare the hook subprocess. Attach a pipe to its stdin, and merge
    # both its stdout and stderr into our own stdout.
    stdin_reader, stdin_writer = IO.pipe
    hook_pid = spawn(vars, hook, in: stdin_reader, err: :out)
    stdin_reader.close

    # Submit changes to the hook via its stdin.
    begin
      IO.copy_stream(StringIO.new(changes), stdin_writer)
    rescue Errno::EPIPE # rubocop:disable Lint/HandleExceptions
      # It is not an error if the hook does not consume all of its input.
    end

    # Close the pipe to let the hook know there is no further input.
    stdin_writer.close

    Process.wait(hook_pid)
    $?.success?
  end

  # lookup hook files in this order:
  #
  # 1. <repository>.git/custom_hooks/<hook_name> - per project hook
  # 2. <repository>.git/custom_hooks/<hook_name>.d/* - per project hooks
  # 3. <repository>.git/hooks/<hook_name>.d/* - global hooks
  #
  def find_hooks(hook_name)
    hook_files = []

    # <repository>.git/custom_hooks/<hook_name>
    project_custom_hook_file = File.join(@repo_path, 'custom_hooks', hook_name)
    hook_files.push(project_custom_hook_file) if File.executable?(project_custom_hook_file)

    # <repository>.git/custom_hooks/<hook_name>.d/*
    project_custom_hooks_dir = File.join(@repo_path, 'custom_hooks', "#{hook_name}.d")
    hook_files += match_hook_files(project_custom_hooks_dir)

    # <repository>.git/hooks/<hook_name>.d/* OR <custom_hook_dir>/<hook_name>.d/*
    global_custom_hooks_parent = config.custom_hooks_dir(default: File.join(@repo_path, 'hooks'))
    global_custom_hooks_dir = File.join(global_custom_hooks_parent, "#{hook_name}.d")
    hook_files += match_hook_files(global_custom_hooks_dir)

    hook_files
  end

  # match files from path:
  # 1. file must be executable
  # 2. file must not match backup file
  #
  # the resulting list is sorted
  def match_hook_files(path)
    return [] unless Dir.exist?(path)

    Dir["#{path}/*"].select do |f|
      !f.end_with?('~') && File.executable?(f)
    end.sort
  end
end
