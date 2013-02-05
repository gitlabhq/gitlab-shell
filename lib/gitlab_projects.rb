require 'open3'
require 'fileutils'
require_relative 'gitlab_config'

class GitlabProjects
  attr_reader :project_name, :full_path

  def initialize
    @command = ARGV.shift
    @project_name = ARGV.shift
    @repos_path = GitlabConfig.new.repos_path
    @full_path = File.join(@repos_path, @project_name)
    @hook_path = File.join(ROOT_PATH, 'hooks', 'post-receive')
  end

  def exec
    case @command
    when 'add-project'; add_project
    when 'rm-project';  rm_project
    else
      puts 'not allowed'
    end
  end

  protected

  def add_project
    FileUtils.mkdir_p(full_path, mode: 0770)
    cmd = "cd #{full_path} && git init --bare && ln -s #{@hook_path} #{full_path}/hooks/post-receive"
    system(cmd)
  end

  def rm_project
    FileUtils.rm_rf(full_path)
  end
end
