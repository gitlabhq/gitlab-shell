require_relative 'spec_helper'
require_relative '../lib/gitlab_projects'

describe GitlabProjects do
  before do
    FileUtils.mkdir_p(tmp_repos_path)
  end

  after do
    FileUtils.rm_rf(tmp_repos_path)
  end

  describe :initialize do
    before do
      argv('add-project', repo_name)
      @gl_projects = GitlabProjects.new
    end

    it { @gl_projects.project_name.should == repo_name }
    it { @gl_projects.instance_variable_get(:@command).should == 'add-project' }
    it { @gl_projects.instance_variable_get(:@full_path).should == '/home/git/repositories/gitlab-ci.git' }
  end

  describe :add_project do
    let(:gl_projects) { build_gitlab_projects('add-project', repo_name) }

    it "should create a directory" do
      gl_projects.stub(system: true)
      gl_projects.exec
      File.exists?(tmp_repo_path).should be_true
    end

    it "should receive valid cmd" do
      valid_cmd = "cd #{tmp_repo_path} && git init --bare"
      valid_cmd << " && ln -s #{ROOT_PATH}/hooks/post-receive #{tmp_repo_path}/hooks/post-receive"
      valid_cmd << " && ln -s #{ROOT_PATH}/hooks/update #{tmp_repo_path}/hooks/update"
      gl_projects.should_receive(:system).with(valid_cmd)
      gl_projects.exec
    end
  end

  describe :mv_project do
    let(:gl_projects) { build_gitlab_projects('mv-project', repo_name, 'repo.git') }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
    end

    it "should move a repo directory" do
      File.exists?(tmp_repo_path).should be_true
      gl_projects.exec
      File.exists?(tmp_repo_path).should be_false
      File.exists?(File.join(tmp_repos_path, 'repo.git')).should be_true
    end
  end

  describe :rm_project do
    let(:gl_projects) { build_gitlab_projects('rm-project', repo_name) }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
    end

    it "should remove a repo directory" do
      File.exists?(tmp_repo_path).should be_true
      gl_projects.exec
      File.exists?(tmp_repo_path).should be_false
    end
  end

  describe :import_project do
    let(:gl_projects) { build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git') }

    it "should import a repo" do
      gl_projects.exec
      File.exists?(File.join(tmp_repo_path, 'HEAD')).should be_true
    end
  end

  describe :enable_git_protocol do
    let(:gl_projects) { build_gitlab_projects('enable-git-protocol', repo_name) }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
    end

    it "should touch a file in repo directory" do
      gl_projects.exec
      File.exists?("#{tmp_repo_path}/git-daemon-export-ok").should be_true
    end
  end

  describe :disable_git_protocol do
    let(:gl_projects) { build_gitlab_projects('disable-git-protocol', repo_name) }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
      FileUtils.touch("#{tmp_repo_path}/git-daemon-export-ok")
    end

    it "should touch a file in repo directory" do
      gl_projects.exec
      File.exists?("#{tmp_repo_path}/git-daemon-export-ok").should be_false
    end
  end

  describe :exec do
    it 'should puts message if unknown command arg' do
      gitlab_projects = build_gitlab_projects('edit-project', repo_name)
      gitlab_projects.should_receive(:puts).with('not allowed')
      gitlab_projects.exec
    end
  end

  def build_gitlab_projects(*args)
    argv(*args)
    gl_projects = GitlabProjects.new
    gl_projects.stub(repos_path: tmp_repos_path)
    gl_projects.stub(full_path: tmp_repo_path)
    gl_projects
  end

  def argv(*args)
    args.each_with_index do |arg, i|
      ARGV[i] = arg
    end
  end

  def tmp_repos_path
    File.join(ROOT_PATH, 'tmp', 'repositories')
  end

  def tmp_repo_path
    File.join(tmp_repos_path, repo_name)
  end

  def repo_name
    'gitlab-ci.git'
  end
end
