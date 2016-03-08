require 'open3'

class GitlabCustomHook

  def pre_receive(changes, repo_path)
    hooks = hook_files('pre-receive', repo_path)
    return true if hooks.nil?
    hooks.each do | hook |
      return false if not call_receive_hook(hook, changes)
    end
    true
  end

  def post_receive(changes, repo_path)
    hooks = hook_files('post-receive', repo_path)
    return true if hooks.nil?
    hooks.each do | hook |
      return false if not call_receive_hook(hook, changes)
    end
    true
  end

  def update(ref_name, old_value, new_value, repo_path)
    hooks = hook_files('update', repo_path)
    return true if hooks.nil?
    hooks.each do | hook |
      if not system(hook, ref_name, old_value, new_value)
        return false
      end
    end
    true
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

  def hook_files(hook_type, repo_path)
    hook_files = []
    parent_path = File.dirname repo_path
    hook_path = File.join(parent_path.strip, 'custom_hooks')
    if Dir.exist?("#{hook_path}/#{hook_type}.d")
      hook_files += Dir["#{hook_path}/#{hook_type}.d/*"].sort
    end
    hook_file = "#{hook_path}/#{hook_type}"
    hook_files.push(hook_file) if File.exist?(hook_file)
    hook_path = File.join(repo_path.strip, 'custom_hooks')
    if Dir.exist?("#{hook_path}/#{hook_type}.d")
      hook_files += Dir["#{hook_path}/#{hook_type}.d/*"].sort
    end
    hook_file = "#{hook_path}/#{hook_type}"
    hook_files.push(hook_file) if File.exist?(hook_file)
    hook_files
  end
end
