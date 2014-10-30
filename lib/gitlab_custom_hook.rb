class GitlabCustomHook
  def pre_receive(refs, repo_path)
    if receive('pre-receive', refs, repo_path)
      return true
    else
      # reset GL_ID env since we stop git push here
      ENV['GL_ID'] = nil
      return false
    end
  end

  def post_receive(refs, repo_path)
    receive('post-receive', refs, repo_path)
  end

  def update(ref_name, old_value, new_value, repo_path)
    hook = hook_file('update', repo_path)
    return true if hook.nil?
    system(*hook, ref_name, old_value, new_value) ? true : false
  end

  private

  def receive(type, refs, repo_path)
    unless type == 'pre-receive' || type == 'post-receive'
      puts 'GitLab: An unexpected error occurred ' \
           '(invalid pre/post-receive hook type)'
      return false
    end

    hook = hook_file(type, repo_path)
    return true if hook.nil?
    cmd = "#{hook} #{refs}"
    system(*cmd) ? true : false
  end

  def hook_file(hook_type, repo_path)
    hook_path = File.join(repo_path.strip, 'custom_hooks')
    hook_file = "#{hook_path}/#{hook_type}"
    hook_file if File.exist?(hook_file)
  end
end
