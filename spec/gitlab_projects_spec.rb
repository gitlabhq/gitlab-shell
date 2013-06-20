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
    it { @gl_projects.instance_variable_get(:@full_path).should == "#{GitlabConfig.new.repos_path}/gitlab-ci.git" }
  end

  describe :create_branch do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    let(:gl_projects) { build_gitlab_projects('create-branch', repo_name, 'test_branch', 'master') }

    it "should create a branch" do
      gl_projects_create.exec
      gl_projects.exec
      branch_ref = `cd #{tmp_repo_path} && git rev-parse test_branch`.strip
      master_ref = `cd #{tmp_repo_path} && git rev-parse master`.strip
      branch_ref.should == master_ref
    end
  end

  describe :rm_branch do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    let(:gl_projects_create_branch) {
      build_gitlab_projects('create-branch', repo_name, 'test_branch', 'master')
    }
    let(:gl_projects) { build_gitlab_projects('rm-branch', repo_name, 'test_branch') }

    it "should remove a branch" do
      gl_projects_create.exec
      gl_projects_create_branch.exec
      branch_ref = `cd #{tmp_repo_path} && git rev-parse test_branch`.strip
      gl_projects.exec
      branch_del = `cd #{tmp_repo_path} && git rev-parse test_branch`.strip
      branch_del.should_not == branch_ref
    end
  end

  describe :create_tag do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    let(:gl_projects) { build_gitlab_projects('create-tag', repo_name, 'test_tag', 'master') }

    it "should create a tag" do
      gl_projects_create.exec
      gl_projects.exec
      tag_ref = `cd #{tmp_repo_path} && git rev-parse test_tag`.strip
      master_ref = `cd #{tmp_repo_path} && git rev-parse master`.strip
      tag_ref.should == master_ref
    end
  end

  describe :rm_tag do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    let(:gl_projects_create_tag) {
      build_gitlab_projects('create-tag', repo_name, 'test_tag', 'master')
    }
    let(:gl_projects) { build_gitlab_projects('rm-tag', repo_name, 'test_tag') }

    it "should remove a branch" do
      gl_projects_create.exec
      gl_projects_create_tag.exec
      branch_ref = `cd #{tmp_repo_path} && git rev-parse test_tag`.strip
      gl_projects.exec
      branch_del = `cd #{tmp_repo_path} && git rev-parse test_tag`.strip
      branch_del.should_not == branch_ref
    end
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

  describe :fork_project do
    let(:source_repo_name) { File.join('source-namespace', repo_name) }
    let(:dest_repo) { File.join(tmp_repos_path, 'forked-to-namespace', repo_name) }
    let(:gl_projects_fork) { build_gitlab_projects('fork-project', source_repo_name, 'forked-to-namespace') }
    let(:gl_projects_import) { build_gitlab_projects('import-project', source_repo_name, 'https://github.com/randx/six.git') }

    before do
      gl_projects_import.exec
    end

    it "should not fork into a namespace that doesn't exist" do
      gl_projects_fork.exec.should be_false
    end

    it "should fork the repo" do
      # create destination namespace
      FileUtils.mkdir_p(File.join(tmp_repos_path, 'forked-to-namespace'))
      gl_projects_fork.exec.should be_true
      File.exists?(dest_repo).should be_true
      File.exists?(File.join(dest_repo, '/hooks/update')).should be_true
      File.exists?(File.join(dest_repo, '/hooks/post-receive')).should be_true
    end

    it "should not fork if a project of the same name already exists" do
      #trying to fork again should fail as the repo already exists
      gl_projects_fork.exec.should be_false
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
    gl_projects.stub(full_path: File.join(tmp_repos_path, gl_projects.project_name))
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
