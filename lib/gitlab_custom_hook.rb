require 'open3'
require_relative 'gitlab_init'
require_relative 'gitlab_metrics'

class GitlabCustomHook
  attr_reader :vars

  def initialize(repo_path, key_id)
    @repo_path = repo_path
    @vars = { 'GL_ID' => key_id }
  end

  def pre_receive(changes)
    hook = hook_file('pre-receive', @repo_path)
    return true if hook.nil?

    GitlabMetrics.measure("pre-receive-hook") { call_receive_hook(hook, changes) }
  end

  def post_receive(changes)
    hook = hook_file('post-receive', @repo_path)
    return true if hook.nil?

    GitlabMetrics.measure("post-receive-hook") { call_receive_hook(hook, changes) }
  end

  def update(ref_name, old_value, new_value)
    hook = hook_file('update', @repo_path)
    return true if hook.nil?

    GitlabMetrics.measure("update-hook") { system(vars, hook, ref_name, old_value, new_value) }
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
    rescue Errno::EPIPE
      # It is not an error if the hook does not consume all of its input.
    end

    # Close the pipe to let the hook know there is no further input.
    stdin_writer.close

    Process.wait(hook_pid)
    $?.success?
  end

  def hook_file(hook_type, repo_path)
    hook_path = File.join(repo_path.strip, 'custom_hooks')
    hook_file = "#{hook_path}/#{hook_type}"
    hook_file if File.executable?(hook_file)
  end
end
