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
    when 'init-project'; init_project
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
    cmd = %W(git --git-dir=#{full_path} tag -- #{tag_name} #{ref})
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
    system(*cmd) && create_hooks(full_path)
  end

  def init_project
    tmp_path = ARGV.shift
    init_from_template = ARGV.shift
    config_auto_init_template_dir = ARGV.shift
    username = ARGV.shift
    user_email = ARGV.shift
    $logger.info "Initializing project"

    # create the tmp directory
    FileUtils.mkdir_p(tmp_path, mode: 0770)

    # set some git config stuff
    cmd = "git config --global user.name '#{username}'"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"
    cmd = "git config --global user.email '#{user_email}'"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    # change workind dir to tmp_path to init the repo
    FileUtils.cd(tmp_path) do
      # if user (which installed gitlab) created the autoinit-template-dir, copy everything
      if init_from_template == "true"
        FileUtils.cp_r(Dir.glob("#{config_auto_init_template_dir}/*"), tmp_path)
      # else create a simple README.md file
      else
        FileUtils.touch "README.md"
        begin
          file = File.open("README.md", "w")
          file.write("# README")
          file.write("\n")
          file.write("This file was automatically created by GitLab.")
          file.write("\n")
        rescue IOError => e
          $logger.warn "Could not write to README.md"
        ensure
          file.close unless file == nil
        end
      end

      # add everything to the repo and push it into the repo
      git_init_repo(tmp_path)
    end
  end

  def git_init_repo(tmp_path)
    cmd = "git init"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    cmd = "git add ."
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    cmd = "git commit -m 'initial commit by GitLab'"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    cmd = "git remote add origin #{full_path}"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    cmd = "git push -u origin master"
    result = system(*cmd)
    $logger.info "#{cmd} #{result}"

    FileUtils.rm_rf("#{tmp_path}")
  end

  def create_hooks(path)
    hook = File.join(path, 'hooks', 'update')
    File.delete(hook) if File.exists?(hook)
    File.symlink(File.join(ROOT_PATH, 'hooks', 'update'), hook)
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
    cmd = %W(git clone --bare -- #{@source} #{full_path})
    system(*cmd) && create_hooks(full_path)
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
    system(*cmd) && create_hooks(full_destination_path)
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
