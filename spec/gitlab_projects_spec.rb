require_relative 'spec_helper'
require_relative '../lib/gitlab_projects'

describe GitlabProjects do
  before do
    FileUtils.mkdir_p(tmp_repos_path)
    $logger = double('logger').as_null_object
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

    it "should log an add-project event" do
      $logger.should_receive(:info).with("Adding project #{repo_name} at <#{tmp_repo_path}>.")
      gl_projects.exec
    end
  end

  describe :mv_project do
    let(:gl_projects) { build_gitlab_projects('mv-project', repo_name, 'repo.git') }
    let(:new_repo_path) { File.join(tmp_repos_path, 'repo.git') }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
    end

    it "should move a repo directory" do
      File.exists?(tmp_repo_path).should be_true
      gl_projects.exec
      File.exists?(tmp_repo_path).should be_false
      File.exists?(new_repo_path).should be_true
    end

    it "should fail if no destination path is provided" do
      incomplete = build_gitlab_projects('mv-project', repo_name)
      $logger.should_receive(:error).with("mv-project failed: no destination path provided.")
      incomplete.exec.should be_false
    end

    it "should fail if the source path doesn't exist" do
      bad_source = build_gitlab_projects('mv-project', 'bad-src.git', 'dest.git')
      $logger.should_receive(:error).with("mv-project failed: source path <#{tmp_repos_path}/bad-src.git> does not exist.")
      bad_source.exec.should be_false
    end

    it "should fail if the destination path already exists" do
      FileUtils.mkdir_p(File.join(tmp_repos_path, 'already-exists.git'))
      bad_dest = build_gitlab_projects('mv-project', repo_name, 'already-exists.git')
      message = "mv-project failed: destination path <#{tmp_repos_path}/already-exists.git> already exists."
      $logger.should_receive(:error).with(message)
      bad_dest.exec.should be_false
    end

    it "should log an mv-project event" do
      message = "Moving project #{repo_name} from <#{tmp_repo_path}> to <#{new_repo_path}>."
      $logger.should_receive(:info).with(message)
      gl_projects.exec
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

    it "should log an rm-project event" do
      $logger.should_receive(:info).with("Removing project #{repo_name} from <#{tmp_repo_path}>.")
      gl_projects.exec
    end
  end

  describe :import_project do
    let(:gl_projects) { build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git') }

    it "should import a repo" do
      gl_projects.exec
      File.exists?(File.join(tmp_repo_path, 'HEAD')).should be_true
    end

    it "should log an import-project event" do
      message = "Importing project #{repo_name} from <https://github.com/randx/six.git> to <#{tmp_repo_path}>."
      $logger.should_receive(:info).with(message)
      gl_projects.exec
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

    it "should not fork without a destination namespace" do
      missing_arg = build_gitlab_projects('fork-project', source_repo_name)
      $logger.should_receive(:error).with("fork-project failed: no destination namespace provided.")
      missing_arg.exec.should be_false
    end

    it "should not fork into a namespace that doesn't exist" do
      message = "fork-project failed: destination namespace <#{tmp_repos_path}/forked-to-namespace> does not exist."
      $logger.should_receive(:error).with(message)
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
      # create a fake project at the intended destination
      FileUtils.mkdir_p(File.join(tmp_repos_path, 'forked-to-namespace', repo_name))

      # trying to fork again should fail as the repo already exists
      message = "fork-project failed: destination repository <#{tmp_repos_path}/forked-to-namespace/#{repo_name}> "
      message << "already exists."
      $logger.should_receive(:error).with(message)
      gl_projects_fork.exec.should be_false
    end

    it "should log a fork-project event" do
      message = "Forking project from <#{File.join(tmp_repos_path, source_repo_name)}> to <#{dest_repo}>."
      $logger.should_receive(:info).with(message)

      # create destination namespace
      FileUtils.mkdir_p(File.join(tmp_repos_path, 'forked-to-namespace'))
      gl_projects_fork.exec.should be_true
    end
  end

  describe :exec do
    it 'should puts message if unknown command arg' do
      gitlab_projects = build_gitlab_projects('edit-project', repo_name)
      gitlab_projects.should_receive(:puts).with('not allowed')
      gitlab_projects.exec
    end

    it 'should log a warning for unknown commands' do
      gitlab_projects = build_gitlab_projects('hurf-durf', repo_name)
      $logger.should_receive(:warn).with('Attempt to execute invalid gitlab-projects command "hurf-durf".')
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
