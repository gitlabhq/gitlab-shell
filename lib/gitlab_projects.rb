require 'open3'
require 'fileutils'

require_relative 'gitlab_config'

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
    when 'add-project'; add_project
    when 'rm-project';  rm_project
    when 'mv-project';  mv_project
    when 'import-project'; import_project
    when 'fork-project'; fork_project
    else
      puts 'not allowed'
      false
    end
  end

  protected

  def add_project
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = "cd #{full_path} && git init --bare && #{create_hooks_cmd}"
    system(cmd)
  end

  def create_hooks_cmd
    create_hooks_to(full_path)
  end

  def rm_project
    FileUtils.rm_rf(full_path)
  end

  # Import project via git clone --bare
  # URL must be publicly clonable
  def import_project
    @source = ARGV.shift
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

    return false unless new_path

    new_full_path = File.join(repos_path, new_path)

    # check if source repo exists
    # and target repo does not exist
    return false unless File.exists?(full_path)
    return false if File.exists?(new_full_path)

    FileUtils.mv(full_path, new_full_path)
  end

  def fork_project
    new_namespace = ARGV.shift

    # destination namespace must be provided
    return false unless new_namespace

    #destination namespace must exist
    namespaced_path = File.join(repos_path, new_namespace)
    return false unless File.exists?(namespaced_path)

    #a project of the same name cannot already be within the destination namespace
    full_destination_path = File.join(namespaced_path, project_name.split('/')[-1])
    return false if File.exists?(full_destination_path)

    cmd = "cd #{namespaced_path} && git clone --bare #{full_path} && #{create_hooks_to(full_destination_path)}"
    system(cmd)
  end

  private

  def create_hooks_to(dest_path)
    pr_hook_path = File.join(ROOT_PATH, 'hooks', 'post-receive')
    up_hook_path = File.join(ROOT_PATH, 'hooks', 'update')

    "ln -s #{pr_hook_path} #{dest_path}/hooks/post-receive && ln -s #{up_hook_path} #{dest_path}/hooks/update"

  end

end
