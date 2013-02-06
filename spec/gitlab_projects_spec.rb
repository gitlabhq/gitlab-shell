require_relative 'spec_helper'
require_relative '../lib/gitlab_projects'

describe GitlabProjects do
  describe :initialize do
    before do
      argv('add-project', 'gitlab-ci.git')
      @gl_projects = GitlabProjects.new
    end

    it { @gl_projects.project_name.should == 'gitlab-ci.git' }
    it { @gl_projects.instance_variable_get(:@command).should == 'add-project' }
    it { @gl_projects.instance_variable_get(:@full_path).should == '/home/git/repositories/gitlab-ci.git' }
  end

  describe :add_project do
    before do
      argv('add-project', 'gitlab-ci.git')
      @gl_projects = GitlabProjects.new
      @gl_projects.stub(full_path: tmp_repo_path)
    end

    after do
      FileUtils.rm_rf(tmp_repo_path)
    end

    it "should create a directory" do
      @gl_projects.stub(system: true)
      @gl_projects.send :add_project
      File.exists?(tmp_repo_path).should be_true
    end

    it "should receive valid cmd" do
      valid_cmd = "cd #{tmp_repo_path} && git init --bare"
      valid_cmd << " && ln -s #{ROOT_PATH}/hooks/post-receive #{tmp_repo_path}/hooks/post-receive"
      valid_cmd << " && ln -s #{ROOT_PATH}/hooks/update #{tmp_repo_path}/hooks/update"
      @gl_projects.should_receive(:system).with(valid_cmd)
      @gl_projects.send :add_project
    end
  end

  def argv(*args)
    args.each_with_index do |arg, i|
      ARGV[i] = arg
    end
  end

  def tmp_repo_path
    File.join(ROOT_PATH, 'tmp', 'gitlab-ci.git')
  end
end
