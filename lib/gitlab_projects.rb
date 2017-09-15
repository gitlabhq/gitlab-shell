require 'fileutils'
require 'tempfile'
require 'timeout'
require 'open3'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_metrics'

class GitlabProjects
  GLOBAL_HOOKS_DIRECTORY = File.join(ROOT_PATH, 'hooks')

  # Project name is a directory name for repository with .git at the end
  # It may be namespaced or not. Like repo.git or gitlab/repo.git
  attr_reader :project_name

  # Absolute path to directory where repositories stored
  # By default it is /home/git/repositories
  attr_reader :repos_path

  # Full path is an absolute path to the repository
  # Ex /home/git/repositories/test.git
  attr_reader :full_path

  def self.create_hooks(path)
    local_hooks_directory = File.join(path, 'hooks')
    real_local_hooks_directory = :not_found
    begin
      real_local_hooks_directory = File.realpath(local_hooks_directory)
    rescue Errno::ENOENT
      # real_local_hooks_directory == :not_found
    end

    if real_local_hooks_directory != File.realpath(GLOBAL_HOOKS_DIRECTORY)
      if File.exist?(local_hooks_directory)
        $logger.info "Moving existing hooks directory and symlinking global hooks directory for #{path}."
        FileUtils.mv(local_hooks_directory, "#{local_hooks_directory}.old.#{Time.now.to_i}")
      end
      FileUtils.ln_sf(GLOBAL_HOOKS_DIRECTORY, local_hooks_directory)
    else
      $logger.info "Hooks already exist for #{path}."
      true
    end
  end

  def initialize
    @command = ARGV.shift
    @repos_path = ARGV.shift
    @project_name = ARGV.shift
    @full_path = File.join(@repos_path, @project_name) unless @project_name.nil?
  end

  def exec
    GitlabMetrics.measure("command-#{@command}") do
      case @command
      when 'create-tag';
        create_tag
      when 'add-project';
        add_project
      when 'list-projects';
        puts list_projects
      when 'rm-project';
        rm_project
      when 'mv-project';
        mv_project
      when 'mv-storage';
        mv_storage
      when 'import-project';
        import_project
      when 'fork-project';
        fork_project
      when 'fetch-remote';
        fetch_remote
      when 'push-branches';
        push_branches
      when 'delete-remote-branches';
        delete_remote_branches
      when 'list-remote-tags';
        list_remote_tags
      when 'gc';
        gc
      else
        $logger.warn "Attempt to execute invalid gitlab-projects command #{@command.inspect}."
        puts 'not allowed'
        false
      end
    end
  end

  protected

  def list_remote_tags
    remote_name = ARGV.shift

    tag_list, exit_code, error = nil
    cmd = %W(git --git-dir=#{full_path} ls-remote --tags #{remote_name})

    Open3.popen3(*cmd) do |stdin, stdout, stderr, wait_thr|
      tag_list  = stdout.read
      error     = stderr.read
      exit_code = wait_thr.value.exitstatus
    end

    if exit_code.zero?
      puts tag_list
      true
    else
      puts error
      false
    end
  end

  def push_branches
    remote_name = ARGV.shift

    # timeout for push
    timeout = (ARGV.shift || 120).to_i

    # push with --force?
    forced = ARGV.delete('--force') if ARGV.include?('--force')

    $logger.info "Pushing branches from #{full_path} to remote #{remote_name}: #{ARGV}"
    cmd = %W(git --git-dir=#{full_path} push)
    cmd << forced if forced
    cmd += %W(-- #{remote_name}).concat(ARGV)
    pid = Process.spawn(*cmd)

    begin
      Timeout.timeout(timeout) do
        Process.wait(pid)
      end

      $?.exitstatus.zero?
    rescue => exception
      $logger.error "Pushing branches to remote #{remote_name} failed due to: #{exception.message}."

      Process.kill('KILL', pid)
      Process.wait
      false
    end
  end

  def delete_remote_branches
    remote_name = ARGV.shift
    branches = ARGV.map { |branch_name| ":#{branch_name}" }

    $logger.info "Pushing deleted branches from #{full_path} to remote #{remote_name}: #{ARGV}"
    cmd = %W(git --git-dir=#{full_path} push -- #{remote_name}).concat(branches)
    pid = Process.spawn(*cmd)

    begin
      Process.wait(pid)

      $?.exitstatus.zero?
    rescue => exception
      $logger.error "Pushing deleted branches to remote #{remote_name} failed due to: #{exception.message}"

      Process.kill('KILL', pid)
      Process.wait
      false
    end
  end

  def create_tag
    tag_name = ARGV.shift
    ref = ARGV.shift || "HEAD"
    cmd = %W(git --git-dir=#{full_path} tag)
    if ARGV.size > 0
      msg = ARGV.shift
      cmd += %W(-a -m #{msg})
    end
    cmd += %W(-- #{tag_name} #{ref})
    system(*cmd)
  end

  def add_project
    $logger.info "Adding project #{@project_name} at <#{full_path}>."
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = %W(git --git-dir=#{full_path} init --bare)
    system(*cmd) && self.class.create_hooks(full_path)
  end

  def list_projects
    $logger.info 'Listing projects'
    Dir.chdir(repos_path) do
      next Dir.glob('**/*.git')
    end
  end

  def rm_project
    $logger.info "Removing project #{@project_name} from <#{full_path}>."
    FileUtils.rm_rf(full_path)
  end

  def mask_password_in_url(url)
    result = URI(url)
    result.password = "*****" unless result.password.nil?
    result.user = "*****" unless result.user.nil? #it's needed for oauth access_token
    result
  rescue
    url
  end

  def fetch_remote
    @name = ARGV.shift

    # timeout for fetch
    timeout = (ARGV.shift || 120).to_i

    # fetch with --force ?
    forced = ARGV.include?('--force')

    # fetch with --tags or --no-tags
    tags_option = ARGV.include?('--no-tags') ? '--no-tags' : '--tags'

    $logger.info "Fetching remote #{@name} for project #{@project_name}."
    cmd = %W(git --git-dir=#{full_path} fetch #{@name} --prune --quiet)
    cmd << '--force' if forced
    cmd << tags_option

    setup_ssh_auth do |env|
      pid = Process.spawn(env, *cmd)

      begin
        _, status = Timeout.timeout(timeout) do
          Process.wait2(pid)
        end

        status.success?
      rescue => exception
        $logger.error "Fetching remote #{@name} for project #{@project_name} failed due to: #{exception.message}."

        Process.kill('KILL', pid)
        Process.wait
        false
      end
    end
  end

  def remove_origin_in_repo
    cmd = %W(git --git-dir=#{full_path} remote rm origin)
    pid = Process.spawn(*cmd)
    Process.wait(pid)
  end

  # Import project via git clone --bare
  # URL must be publicly cloneable
  def import_project
    # Skip import if repo already exists
    return false if File.exists?(full_path)

    @source = ARGV.shift
    masked_source = mask_password_in_url(@source)

    # timeout for clone
    timeout = (ARGV.shift || 120).to_i
    $logger.info "Importing project #{@project_name} from <#{masked_source}> to <#{full_path}>."
    cmd = %W(git clone --bare -- #{@source} #{full_path})

    pid = Process.spawn(*cmd)

    begin
      Timeout.timeout(timeout) do
        Process.wait(pid)
      end

      return false unless $?.exitstatus.zero?
    rescue Timeout::Error
      $logger.error "Importing project #{@project_name} from <#{masked_source}> failed due to timeout."

      Process.kill('KILL', pid)
      Process.wait
      FileUtils.rm_rf(full_path)
      return false
    end

    self.class.create_hooks(full_path)
    # The project was imported successfully.
    # Remove the origin URL since it may contain password.
    remove_origin_in_repo

    true
  end

  # Move repository from one directory to another
  #
  # Ex.
  #  gitlab.git -> gitlabhq.git
  #  gitlab/gitlab-ci.git -> randx/six.git
  #
  # Wont work if target namespace directory does not exist
  #
  def mv_project
    new_path = ARGV.shift

    unless new_path
      $logger.error "mv-project failed: no destination path provided."
      return false
    end

    new_full_path = File.join(repos_path, new_path)

    # verify that the source repo exists
    unless File.exists?(full_path)
      $logger.error "mv-project failed: source path <#{full_path}> does not exist."
      return false
    end

    # ...and that the target repo does not exist
    if File.exists?(new_full_path)
      $logger.error "mv-project failed: destination path <#{new_full_path}> already exists."
      return false
    end

    $logger.info "Moving project #{@project_name} from <#{full_path}> to <#{new_full_path}>."
    FileUtils.mv(full_path, new_full_path)
  end

  # Move repository from one storage path to another
  #
  # Wont work if target namespace directory does not exist in the new storage path
  #
  def mv_storage
    new_storage = ARGV.shift

    unless new_storage
      $logger.error "mv-storage failed: no destination storage path provided."
      return false
    end

    new_full_path = File.join(new_storage, project_name)

    # verify that the source repo exists
    unless File.exists?(full_path)
      $logger.error "mv-storage failed: source path <#{full_path}> does not exist."
      return false
    end

    # Make sure the destination directory exists
    FileUtils.mkdir_p(new_full_path)

    # Make sure the source path ends with a slash so that rsync copies the
    # contents of the directory, as opposed to copying the directory by name
    source_path = File.join(full_path, '')

    if wait_for_pushes
      $logger.info "Syncing project #{@project_name} from <#{full_path}> to <#{new_full_path}>."

      # Set a low IO priority with ionice to not choke the server on moves
      if rsync(source_path, new_full_path, 'ionice -c2 -n7 rsync')
        true
      else
        # If the command fails with `ionice` (maybe because we're on a OS X
        # development machine), try again without `ionice`.
        rsync(source_path, new_full_path)
      end
    else
      $logger.error "mv-storage failed: source path <#{full_path}> is waiting for pushes to finish."
      false
    end
  end

  def fork_project
    destination_repos_path = ARGV.shift

    unless destination_repos_path
      $logger.error "fork-project failed: no destination repository path provided."
      return false
    end

    new_namespace = ARGV.shift

    # destination namespace must be provided
    unless new_namespace
      $logger.error "fork-project failed: no destination namespace provided."
      return false
    end

    # destination namespace must exist
    namespaced_path = File.join(destination_repos_path, new_namespace)
    unless File.exists?(namespaced_path)
      $logger.error "fork-project failed: destination namespace <#{namespaced_path}> does not exist."
      return false
    end

    # a project of the same name cannot already be within the destination namespace
    full_destination_path = File.join(namespaced_path, project_name.split('/')[-1])
    if File.exists?(full_destination_path)
      $logger.error "fork-project failed: destination repository <#{full_destination_path}> already exists."
      return false
    end

    $logger.info "Forking project from <#{full_path}> to <#{full_destination_path}>."
    cmd = %W(git clone --bare -- #{full_path} #{full_destination_path})
    system(*cmd) && self.class.create_hooks(full_destination_path)
  end

  def gc
    $logger.info "Running git gc for <#{full_path}>."
    unless File.exists?(full_path)
      $logger.error "gc failed: destination path <#{full_path}> does not exist."
      return false
    end
    cmd = %W(git --git-dir=#{full_path} gc)
    system(*cmd)
  end

  def wait_for_pushes
    # Try for 30 seconds, polling every 10
    3.times do
      return true if gitlab_reference_counter.value == 0
      sleep 10
    end

    false
  end

  # Builds a small shell script that can be used to execute SSH with a set of
  # custom options.
  #
  # Options are expanded as `'-oKey="Value"'`, so SSH will correctly interpret
  # paths with spaces in them. We trust the user not to embed single or double
  # quotes in the key or value.
  def custom_ssh_script(options = {})
    args = options.map { |k, v| "'-o#{k}=\"#{v}\"'" }.join(' ')

    [
      "#!/bin/sh",
      "exec ssh #{args} \"$@\""
    ].join("\n")
  end

  # Known hosts data and private keys can be passed to gitlab-shell in the
  # environment. If present, this method puts them into temporary files, writes
  # a script that can substitute as `ssh`, setting the options to respect those
  # files, and yields: { "GIT_SSH" => "/tmp/myScript" }
  def setup_ssh_auth
    options = {}

    if ENV.key?('GITLAB_SHELL_SSH_KEY')
      key_file = Tempfile.new('gitlab-shell-key-file')
      key_file.chmod(0o400)
      key_file.write(ENV['GITLAB_SHELL_SSH_KEY'])
      key_file.close

      options['IdentityFile'] = key_file.path
      options['IdentitiesOnly'] = 'yes'
    end

    if ENV.key?('GITLAB_SHELL_KNOWN_HOSTS')
      known_hosts_file = Tempfile.new('gitlab-shell-known-hosts')
      known_hosts_file.chmod(0o400)
      known_hosts_file.write(ENV['GITLAB_SHELL_KNOWN_HOSTS'])
      known_hosts_file.close

      options['StrictHostKeyChecking'] = 'yes'
      options['UserKnownHostsFile'] = known_hosts_file.path
    end

    return yield({}) if options.empty?

    script = Tempfile.new('gitlab-shell-ssh-wrapper')
    script.chmod(0o755)
    script.write(custom_ssh_script(options))
    script.close

    yield('GIT_SSH' => script.path)
  ensure
    key_file.close! unless key_file.nil?
    known_hosts_file.close! unless known_hosts_file.nil?
    script.close! unless script.nil?
  end

  def gitlab_reference_counter
    @gitlab_reference_counter ||= begin
      # Defer loading because this pulls in gitlab_net, which takes 100-200 ms
      # to load
      require_relative 'gitlab_reference_counter'
      GitlabReferenceCounter.new(full_path)
    end
  end

  def rsync(src, dest, rsync_path = 'rsync')
    command = rsync_path.split + %W(-a --delete --rsync-path="#{rsync_path}" #{src} #{dest})
    system(*command)
  end
end
