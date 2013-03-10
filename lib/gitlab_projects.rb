require 'open3'
require 'fileutils'

class GitlabProjects
  attr_reader :project_name, :full_path

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
    when 'import-project'; import_project
    else
      puts 'not allowed'
    end
  end

  protected

  def add_project
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = "cd #{full_path} && git init --bare && #{create_hooks_cmd}"
    system(cmd)
  end

  def create_hooks_cmd
    pr_hook_path = File.join(ROOT_PATH, 'hooks', 'post-receive')
    up_hook_path = File.join(ROOT_PATH, 'hooks', 'update')

    "ln -s #{pr_hook_path} #{full_path}/hooks/post-receive && ln -s #{up_hook_path} #{full_path}/hooks/update"
  end

  def rm_project
    FileUtils.rm_rf(full_path)
  end

  def import_project
    @source = ARGV.shift
    cmd = "cd #{@repos_path} && git clone --bare #{@source} #{@project_name} && #{create_hooks_cmd}"
    system(cmd)
  end
end
