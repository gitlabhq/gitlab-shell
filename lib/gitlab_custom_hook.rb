require 'open3'

class GitlabCustomHook
  def pre_receive(changes, repo_path)
    hook = hook_file('pre-receive', repo_path)
    return true if hook.nil?

    call_receive_hook(hook, changes)
  end

  def post_receive(changes, repo_path)
    hook = hook_file('post-receive', repo_path)
    return true if hook.nil?
    
    call_receive_hook(hook, changes)
  end

  def update(ref_name, old_value, new_value, repo_path)
    hook = hook_file('update', repo_path)
    return true if hook.nil?

    system(hook, ref_name, old_value, new_value)
  end

  private

  def call_receive_hook(hook, changes)
    # Prepare the hook subprocess. Attach a pipe to its stdin, and merge
    # both its stdout and stderr into our own stdout.
    stdin_reader, stdin_writer = IO.pipe
    hook_pid = spawn(hook, in: stdin_reader, err: :out)
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
    return nil if hook_type == nil
    search_path = [repo_path, ROOT_PATH]
    search_path.each do |hook_path|
      next if hook_path == nil
      hook_file = File.join(hook_path.strip, 'custom_hook', hook_type)
      return hook_file if File.exist?(hook_file)
    end
    nil
  end
end
