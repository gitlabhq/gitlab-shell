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

  describe :create_branch do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    let(:gl_projects) { build_gitlab_projects('create-branch', repo_name, 'test_branch', 'master') }

    it "should create a branch" do
      gl_projects_create.exec
      gl_projects.exec
      branch_ref = capture_in_tmp_repo(%W(git rev-parse test_branch))
      master_ref = capture_in_tmp_repo(%W(git rev-parse master))
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
      branch_ref = capture_in_tmp_repo(%W(git rev-parse test_branch))
      gl_projects.exec
      branch_del = capture_in_tmp_repo(%W(git rev-parse test_branch))
      branch_del.should_not == branch_ref
    end
  end

  describe :create_tag do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git')
    }
    context "lightweight tag" do
      let(:gl_projects) { build_gitlab_projects('create-tag', repo_name, 'test_tag', 'master') }

      it "should create a tag" do
        gl_projects_create.exec
        gl_projects.exec
        tag_ref = capture_in_tmp_repo(%W(git rev-parse test_tag))
        master_ref = capture_in_tmp_repo(%W(git rev-parse master))
        tag_ref.should == master_ref
      end
    end
    context "annotated tag" do
      msg = 'some message'
      tag_name = 'test_annotated_tag'
      let(:gl_projects) { build_gitlab_projects('create-tag', repo_name, tag_name, 'master', msg) }

      it "should create an annotated tag" do
        gl_projects_create.exec
        gl_projects.exec

        tag_ref = capture_in_tmp_repo(%W(git rev-parse #{tag_name}^{}))
        master_ref = capture_in_tmp_repo(%W(git rev-parse master))
        tag_msg = capture_in_tmp_repo(%W(git tag -l -n1 #{tag_name}))

        tag_ref.should == master_ref
        tag_msg.should == tag_name + ' ' + msg
      end
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
      branch_ref = capture_in_tmp_repo(%W(git rev-parse test_tag))
      gl_projects.exec
      branch_del = capture_in_tmp_repo(%W(git rev-parse test_tag))
      branch_del.should_not == branch_ref
    end
  end

  describe :add_project do
    let(:gl_projects) { build_gitlab_projects('add-project', repo_name) }

    it "should create a directory" do
      gl_projects.stub(system: true)
      GitlabProjects.stub(create_hooks: true)
      gl_projects.exec
      File.exists?(tmp_repo_path).should be_true
    end

    it "should receive valid cmd" do
      valid_cmd = ['git', "--git-dir=#{tmp_repo_path}", 'init', '--bare']
      gl_projects.should_receive(:system).with(*valid_cmd).and_return(true)
      GitlabProjects.should_receive(:create_hooks).with(tmp_repo_path)
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

  describe :update_head do
    let(:gl_projects) { build_gitlab_projects('update-head', repo_name, 'stable') }
    let(:gl_projects_fail) { build_gitlab_projects 'update-head', repo_name }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
      system(*%W(git init --bare #{tmp_repo_path}))
      FileUtils.touch(File.join(tmp_repo_path, "refs/heads/stable"))
      File.read(File.join(tmp_repo_path, 'HEAD')).strip.should == 'ref: refs/heads/master'
    end

    it "should update head for repo" do
      gl_projects.exec.should be_true
      File.read(File.join(tmp_repo_path, 'HEAD')).strip.should == 'ref: refs/heads/stable'
    end

    it "should log an update_head event" do
      $logger.should_receive(:info).with("Update head in project #{repo_name} to <stable>.")
      gl_projects.exec
    end

    it "should failed and log an error" do
      $logger.should_receive(:error).with("update-head failed: no branch provided.")
      gl_projects_fail.exec.should be_false
    end
  end

  describe :import_project do
    context 'success import' do
      let(:gl_projects) { build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git') }

      it { gl_projects.exec.should be_true }

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

    context 'already exists' do
      let(:gl_projects) { build_gitlab_projects('import-project', repo_name, 'https://github.com/randx/six.git') }

      it 'should import only once' do
        gl_projects.exec.should be_true
        gl_projects.exec.should be_false
      end
    end

    context 'timeout' do
      let(:gl_projects) { build_gitlab_projects('import-project', repo_name, 'https://github.com/gitlabhq/gitlabhq.git', '1') }

      it { gl_projects.exec.should be_false }

      it "should not import a repo" do
        gl_projects.exec
        File.exists?(File.join(tmp_repo_path, 'HEAD')).should be_false
      end

      it "should log an import-project event" do
        message = "Importing project #{repo_name} from <https://github.com/gitlabhq/gitlabhq.git> failed due to timeout."
        $logger.should_receive(:error).with(message)
        gl_projects.exec
      end
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

  def capture_in_tmp_repo(cmd)
    IO.popen([*cmd, {chdir: tmp_repo_path}]).read.strip
  end
end
