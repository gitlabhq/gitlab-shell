require 'open3'
require 'fileutils'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'

class GitlabProjects
  # Project name is a directory name for repository with .git at the end
  # It may be namespaced or not. Like repo.git or gitlab/repo.git
  attr_reader :project_name

  # Absolute path to directory where repositories stored
  # By default it is /home/git/repositories
  attr_reader :repos_path

  # Full path is an absolute path to the repository
  # Ex /home/git/repositories/test.git
  attr_reader :full_path

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
    cmd = "cd #{full_path} && git branch #{branch_name} #{ref}"
    system(cmd)
  end

  def rm_branch
    branch_name = ARGV.shift
    cmd = "cd #{full_path} && git branch -D #{branch_name}"
    system(cmd)
  end

  def create_tag
    tag_name = ARGV.shift
    ref = ARGV.shift || "HEAD"
    cmd = "cd #{full_path} && git tag #{tag_name} #{ref}"
    system(cmd)
  end

  def rm_tag
    tag_name = ARGV.shift
    cmd = "cd #{full_path} && git tag -d #{tag_name}"
    system(cmd)
  end

  def add_project
    $logger.info "Adding project #{@project_name} at <#{full_path}>."
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = "cd #{full_path} && git init --bare && #{create_hooks_cmd}"
    system(cmd)
  end

  def create_hooks_cmd
    create_hooks_to(full_path)
  end

  def rm_project
    $logger.info "Removing project #{@project_name} from <#{full_path}>."
    FileUtils.rm_rf(full_path)
  end

  # Import project via git clone --bare
  # URL must be publicly cloneable
  def import_project
    @source = ARGV.shift
    $logger.info "Importing project #{@project_name} from <#{@source}> to <#{full_path}>."
    cmd = "cd #{repos_path} && git clone --bare #{@source} #{project_name} && #{create_hooks_cmd}"
    system(cmd)
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
    cmd = "cd #{namespaced_path} && git clone --bare #{full_path} && #{create_hooks_to(full_destination_path)}"
    system(cmd)
  end

  def update_head
    new_head = ARGV.shift

    unless new_head
      $logger.error "update-head failed: no branch provided."
      return false
    end

    unless File.exists?(File.join(full_path, 'refs/heads', new_head))
      $logger.error "update-head failed: specified branch does not exist in ref/heads."
      return false
    end

    File.open(File.join(full_path, 'HEAD'), 'w') do |f|
      f.write("ref: refs/heads/#{new_head}")
    end

    $logger.info "Update head in project #{project_name} to <#{new_head}>."
    true
  end

  private

  def create_hooks_to(dest_path)
    symlink_cmds = Dir[File.join(ROOT_PATH, 'hooks', '*')].sort.map do |hook|
      "ln -s #{hook} #{dest_path}/hooks/"
    end
    symlink_cmds.join(' && ')
  end
end
