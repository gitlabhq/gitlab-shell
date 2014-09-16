require 'fileutils'
require 'timeout'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'

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
    unless File.realpath(local_hooks_directory) == File.realpath(GLOBAL_HOOKS_DIRECTORY)
      FileUtils.mv(local_hooks_directory, "#{local_hooks_directory}.#{Time.now.to_i}")
      FileUtils.ln_s(GLOBAL_HOOKS_DIRECTORY, local_hooks_directory)
    end
  end

  def initialize
    @command = ARGV.shift
    @project_name = ARGV.shift
    @repos_path = GitlabConfig.new.repos_path
    @full_path = File.join(@repos_path, @project_name)
  end

  def exec
    case @command
    when 'create-branch'; create_branch
    when 'rm-branch'; rm_branch
    when 'create-tag'; create_tag
    when 'rm-tag'; rm_tag
    when 'add-project'; add_project
    when 'rm-project';  rm_project
    when 'mv-project';  mv_project
    when 'import-project'; import_project
    when 'fork-project'; fork_project
    when 'update-head';  update_head
    else
      $logger.warn "Attempt to execute invalid gitlab-projects command #{@command.inspect}."
      puts 'not allowed'
      false
    end
  end

  protected

  def create_branch
    branch_name = ARGV.shift
    ref = ARGV.shift || "HEAD"
    cmd = %W(git --git-dir=#{full_path} branch -- #{branch_name} #{ref})
    system(*cmd)
  end

  def rm_branch
    branch_name = ARGV.shift
    cmd = %W(git --git-dir=#{full_path} branch -D #{branch_name})
    system(*cmd)
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

  def rm_tag
    tag_name = ARGV.shift
    cmd = %W(git --git-dir=#{full_path} tag -d #{tag_name})
    system(*cmd)
  end

  def add_project
    $logger.info "Adding project #{@project_name} at <#{full_path}>."
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = %W(git --git-dir=#{full_path} init --bare)
    system(*cmd) && self.class.create_hooks(full_path)
  end

  def rm_project
    $logger.info "Removing project #{@project_name} from <#{full_path}>."
    FileUtils.rm_rf(full_path)
  end

  def mask_password_in_url(url)
    result = URI(url)
    result.password = "*****" unless result.password.nil?
    result
  rescue
    url
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
    rescue Timeout::Error
      $logger.error "Importing project #{@project_name} from <#{masked_source}> failed due to timeout."

      Process.kill('KILL', pid)
      Process.wait
      FileUtils.rm_rf(full_path)
      false
    else
      self.class.create_hooks(full_path)
      # The project was imported successfully.
      # Remove the origin URL since it may contain password.
      remove_origin_in_repo
    end
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

  def fork_project
    new_namespace = ARGV.shift

    # destination namespace must be provided
    unless new_namespace
      $logger.error "fork-project failed: no destination namespace provided."
      return false
    end

    # destination namespace must exist
    namespaced_path = File.join(repos_path, new_namespace)
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

  def update_head
    new_head = ARGV.shift

    unless new_head
      $logger.error "update-head failed: no branch provided."
      return false
    end

    File.open(File.join(full_path, 'HEAD'), 'w') do |f|
      f.write("ref: refs/heads/#{new_head}")
    end

    $logger.info "Update head in project #{project_name} to <#{new_head}>."
    true
  end
end
