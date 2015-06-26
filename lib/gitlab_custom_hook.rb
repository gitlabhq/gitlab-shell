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
    # function  will return true if succesful
    exit_status = false

    # we combine both stdout and stderr as we don't know what stream
    # will be used by the custom hook
    Open3.popen2e(hook) do |stdin, stdout_stderr, wait_thr|
      exit_status = true
      stdin.sync = true

      # in git, pre- and post- receive hooks may just exit without
      # reading stdin. We catch the exception to avoid a broken pipe
      # warning
      begin
        # inject all the changes as stdin to the hook
        changes.lines do |line|
          stdin.puts(line)
        end
      rescue Errno::EPIPE
      end

      # need to close stdin before reading stdout
      stdin.close

      unless wait_thr.value == 0
        exit_status = false
      end

      stdout_stderr.each_line { |line| puts line }
    end

    exit_status
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
