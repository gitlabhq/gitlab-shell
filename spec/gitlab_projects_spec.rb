require_relative 'spec_helper'
require_relative '../lib/gitlab_projects'
require_relative '../lib/gitlab_reference_counter'

describe GitlabProjects do
  before do
    FileUtils.mkdir_p(tmp_repos_path)
    $logger = double('logger').as_null_object
  end

  after do
    FileUtils.rm_rf(tmp_repos_path)
  end

  describe :create_hooks do
    let(:repo_path) { File.join(tmp_repos_path, 'hook-test.git') }
    let(:hooks_dir) { File.join(repo_path, 'hooks') }
    let(:target_hooks_dir) { File.join(ROOT_PATH, 'hooks') }
    let(:existing_target) { File.join(repo_path, 'foobar') }

    before do
      FileUtils.rm_rf(repo_path)
      FileUtils.mkdir_p(repo_path)
    end

    context 'hooks is a directory' do
      let(:existing_file) { File.join(hooks_dir, 'my-file') }

      before do
        FileUtils.mkdir_p(hooks_dir)
        FileUtils.touch(existing_file)
        GitlabProjects.create_hooks(repo_path)
      end

      it { File.readlink(hooks_dir).should == target_hooks_dir }
      it { Dir[File.join(repo_path, "hooks.old.*/my-file")].count.should == 1 }
    end

    context 'hooks is a valid symlink' do
      before do
        FileUtils.mkdir_p existing_target
        File.symlink(existing_target, hooks_dir)
        GitlabProjects.create_hooks(repo_path)
      end

      it { File.readlink(hooks_dir).should == target_hooks_dir }
    end

    context 'hooks is a broken symlink' do
      before do
        FileUtils.rm_f(existing_target)
        File.symlink(existing_target, hooks_dir)
        GitlabProjects.create_hooks(repo_path)
      end

      it { File.readlink(hooks_dir).should == target_hooks_dir }
    end
  end

  describe :initialize do
    before do
      argv('add-project', tmp_repos_path, repo_name)
      @gl_projects = GitlabProjects.new
    end

    it { @gl_projects.project_name.should == repo_name }
    it { @gl_projects.repos_path.should == tmp_repos_path }
    it { @gl_projects.full_path.should == "#{tmp_repos_path}/gitlab-ci.git" }
    it { @gl_projects.instance_variable_get(:@command).should == 'add-project' }
  end

  describe :create_tag do
    let(:gl_projects_create) {
      build_gitlab_projects('import-project', tmp_repos_path, repo_name, 'https://github.com/randx/six.git')
    }
    context "lightweight tag" do
      let(:gl_projects) { build_gitlab_projects('create-tag', tmp_repos_path, repo_name, 'test_tag', 'master') }

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

      let(:gl_projects) { build_gitlab_projects('create-tag', tmp_repos_path, repo_name, tag_name, 'master', msg) }

      it "should create an annotated tag" do
        gl_projects_create.exec
        system(*%W(git --git-dir=#{tmp_repo_path} config user.name Joe))
        system(*%W(git --git-dir=#{tmp_repo_path} config user.email joe@smith.com))
        gl_projects.exec

        tag_ref = capture_in_tmp_repo(%W(git rev-parse #{tag_name}^{}))
        master_ref = capture_in_tmp_repo(%W(git rev-parse master))
        tag_msg = capture_in_tmp_repo(%W(git tag -l -n1 #{tag_name}))

        tag_ref.should == master_ref
        tag_msg.should == tag_name + ' ' + msg
      end
    end
  end

  describe :add_project do
    let(:gl_projects) { build_gitlab_projects('add-project', tmp_repos_path, repo_name) }

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

  describe :list_projects do
    let(:gl_projects) do
      build_gitlab_projects('add-project', tmp_repos_path, "list_test/#{repo_name}")
    end

    before do
      FileUtils.mkdir_p(tmp_repos_path)
    end

    it 'should create projects and list them' do
      GitlabProjects.stub(create_hooks: true)
      gl_projects.exec
      gl_projects.send(:list_projects).should == ["list_test/#{repo_name}"]
    end
  end

  describe :mv_project do
    let(:gl_projects) { build_gitlab_projects('mv-project', tmp_repos_path, repo_name, 'repo.git') }
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
      incomplete = build_gitlab_projects('mv-project', tmp_repos_path, repo_name)
      $logger.should_receive(:error).with("mv-project failed: no destination path provided.")
      incomplete.exec.should be_false
    end

    it "should fail if the source path doesn't exist" do
      bad_source = build_gitlab_projects('mv-project', tmp_repos_path, 'bad-src.git', 'dest.git')
      $logger.should_receive(:error).with("mv-project failed: source path <#{tmp_repos_path}/bad-src.git> does not exist.")
      bad_source.exec.should be_false
    end

    it "should fail if the destination path already exists" do
      FileUtils.mkdir_p(File.join(tmp_repos_path, 'already-exists.git'))
      bad_dest = build_gitlab_projects('mv-project', tmp_repos_path, repo_name, 'already-exists.git')
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
    let(:gl_projects) { build_gitlab_projects('rm-project', tmp_repos_path, repo_name) }

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

  describe :mv_storage do
    let(:alternative_storage_path) { File.join(ROOT_PATH, 'tmp', 'alternative') }
    let(:gl_projects) { build_gitlab_projects('mv-storage', tmp_repos_path, repo_name, alternative_storage_path) }
    let(:new_repo_path) { File.join(alternative_storage_path, repo_name) }

    before do
      FileUtils.mkdir_p(tmp_repo_path)
      FileUtils.mkdir_p(File.join(tmp_repo_path, 'hooks')) # Add some contents to copy
      FileUtils.mkdir_p(alternative_storage_path)
      allow_any_instance_of(GitlabReferenceCounter).to receive(:value).and_return(0)
    end

    after { FileUtils.rm_rf(alternative_storage_path) }

    it "should rsync a repo directory" do
      File.exists?(tmp_repo_path).should be_true
      gl_projects.exec
      File.exists?(new_repo_path).should be_true
      # Make sure the target directory isn't empty (i.e. contents were copied)
      FileUtils.cd(new_repo_path) { Dir['**/*'].length.should_not be(0) }
    end

    it "should attempt rsync with ionice first" do
      expect(gl_projects).to receive(:system).with(
        'ionice', '-c2', '-n7', 'rsync', '-a', '--delete', '--rsync-path="ionice -c2 -n7 rsync"',
        "#{tmp_repo_path}/", new_repo_path
      ).and_return(true)

      gl_projects.exec.should be_true
    end

    it "should attempt rsync without ionice if with ionice fails" do
      expect(gl_projects).to receive(:system).with(
        'ionice', '-c2', '-n7', 'rsync', '-a', '--delete', '--rsync-path="ionice -c2 -n7 rsync"',
        "#{tmp_repo_path}/", new_repo_path
      ).and_return(false)

      expect(gl_projects).to receive(:system).with(
        'rsync', '-a', '--delete', '--rsync-path="rsync"', "#{tmp_repo_path}/", new_repo_path
      ).and_return(true)

      gl_projects.exec.should be_true
    end

    it "should fail if both rsync attempts fail" do
      expect(gl_projects).to receive(:system).with(
        'ionice', '-c2', '-n7', 'rsync', '-a', '--delete', '--rsync-path="ionice -c2 -n7 rsync"',
        "#{tmp_repo_path}/", new_repo_path
      ).and_return(false)

      expect(gl_projects).to receive(:system).with(
        'rsync', '-a', '--delete', '--rsync-path="rsync"', "#{tmp_repo_path}/", new_repo_path
      ).and_return(false)

      gl_projects.exec.should be_false
    end

    it "should fail if no destination path is provided" do
      incomplete = build_gitlab_projects('mv-storage', tmp_repos_path, repo_name)
      $logger.should_receive(:error).with("mv-storage failed: no destination storage path provided.")
      incomplete.exec.should be_false
    end

    it "should fail if the source path doesn't exist" do
      bad_source = build_gitlab_projects('mv-storage', tmp_repos_path, 'bad-src.git', alternative_storage_path)
      $logger.should_receive(:error).with("mv-storage failed: source path <#{tmp_repos_path}/bad-src.git> does not exist.")
      bad_source.exec.should be_false
    end

    it "should fail if there are pushes ongoing" do
      allow_any_instance_of(GitlabReferenceCounter).to receive(:value).and_return(1)
      $logger.should_receive(:error).with("mv-storage failed: source path <#{tmp_repo_path}> is waiting for pushes to finish.")
      gl_projects.exec.should be_false
    end

    it "should log an mv-storage event" do
      message = "Syncing project #{repo_name} from <#{tmp_repo_path}> to <#{new_repo_path}>."
      $logger.should_receive(:info).with(message)
      gl_projects.exec
    end
  end

  describe :push_branches do
    let(:repos_path) { 'current/storage' }
    let(:project_name) { 'project/path.git' }
    let(:full_path) { File.join(repos_path, project_name) }
    let(:remote_name) { 'new/storage' }
    let(:pid) { 1234 }
    let(:branch_name) { 'master' }
    let(:cmd) { %W(git --git-dir=#{full_path} push -- #{remote_name} #{branch_name}) }
    let(:gl_projects) { build_gitlab_projects('push-branches', repos_path, project_name, remote_name, '600', 'master') }

    it 'executes the command' do
      expect(Process).to receive(:spawn).with(*cmd).and_return(pid)
      expect(Process).to receive(:wait).with(pid)

      expect(gl_projects.exec).to be true
    end

    it 'raises timeout' do
      expect(Timeout).to receive(:timeout).with(600).and_raise(Timeout::Error)
      expect(Process).to receive(:spawn).with(*cmd).and_return(pid)
      expect(Process).to receive(:wait)
      expect(Process).to receive(:kill).with('KILL', pid)

      expect(gl_projects.exec).to be false
    end

    context 'with --force' do
      let(:cmd) { %W(git --git-dir=#{full_path} push --force -- #{remote_name} #{branch_name}) }
      let(:gl_projects) { build_gitlab_projects('push-branches', repos_path, project_name, remote_name, '600', '--force', 'master') }

      it 'executes the command' do
        expect(Process).to receive(:spawn).with(*cmd).and_return(pid)
        expect(Process).to receive(:wait).with(pid)

        expect(gl_projects.exec).to be true
      end
    end
  end

  describe :fetch_remote do
    let(:repos_path) { 'current/storage' }
    let(:project_name) { 'project.git' }
    let(:full_path) { File.join(repos_path, project_name) }
    let(:remote_name) { 'new/storage' }
    let(:pid) { 1234 }
    let(:branch_name) { 'master' }

    def stub_spawn(*args, wait: true, success: true)
      expect(Process).to receive(:spawn).with(*args).and_return(pid)
      expect(Process).to receive(:wait2).with(pid).and_return([pid, double(success?: success)]) if wait
    end

    def stub_env(args = {})
      original = ENV.to_h
      args.each { |k, v| ENV[k] = v }
      yield
    ensure
      ENV.replace(original)
    end

    def stub_tempfile(name, filename, opts = {})
      chmod = opts.delete(:chmod)
      file = StringIO.new

      allow(file).to receive(:close!)
      allow(file).to receive(:path).and_return(name)

      expect(Tempfile).to receive(:new).with(filename).and_return(file)
      expect(file).to receive(:chmod).with(chmod) if chmod

      file
    end

    describe 'with default args' do
      let(:gl_projects) { build_gitlab_projects('fetch-remote', repos_path, project_name, remote_name, '600') }
      let(:cmd) { %W(git --git-dir=#{full_path} fetch #{remote_name} --prune --quiet --tags) }

      it 'executes the command' do
        stub_spawn({}, *cmd)

        expect(gl_projects.exec).to be true
      end

      it 'raises timeout' do
        stub_spawn({}, *cmd, wait: false)
        expect(Timeout).to receive(:timeout).with(600).and_raise(Timeout::Error)
        expect(Process).to receive(:kill).with('KILL', pid)

        expect(gl_projects.exec).to be false
      end
    end

    describe 'with --force' do
      let(:gl_projects) { build_gitlab_projects('fetch-remote', repos_path, project_name, remote_name, '600', '--force') }
      let(:env) { {} }
      let(:cmd) { %W(git --git-dir=#{full_path} fetch #{remote_name} --prune --quiet --force --tags) }

      it 'executes the command with forced option' do
        stub_spawn({}, *cmd)

        expect(gl_projects.exec).to be true
      end
    end

    describe 'with --no-tags' do
      let(:gl_projects) { build_gitlab_projects('fetch-remote', repos_path, project_name, remote_name, '600', '--no-tags') }
      let(:cmd) { %W(git --git-dir=#{full_path} fetch #{remote_name} --prune --quiet --no-tags) }

      it 'executes the command' do
        stub_spawn({}, *cmd)

        expect(gl_projects.exec).to be true
      end
    end

    describe 'with GITLAB_SHELL_SSH_KEY' do
      let(:gl_projects) { build_gitlab_projects('fetch-remote', repos_path, project_name, remote_name, '600') }
      let(:cmd) { %W(git --git-dir=#{full_path} fetch #{remote_name} --prune --quiet --tags) }

      around(:each) do |example|
        stub_env('GITLAB_SHELL_SSH_KEY' => 'SSH KEY') { example.run }
      end

      it 'sets GIT_SSH to a custom script' do
        script = stub_tempfile('scriptFile', 'gitlab-shell-ssh-wrapper', chmod: 0o755)
        key = stub_tempfile('/tmp files/keyFile', 'gitlab-shell-key-file', chmod: 0o400)

        stub_spawn({ 'GIT_SSH' => 'scriptFile' }, *cmd)

        expect(gl_projects.exec).to be true

        expect(script.string).to eq("#!/bin/sh\nexec ssh '-oIdentityFile=\"/tmp files/keyFile\"' '-oIdentitiesOnly=\"yes\"' \"$@\"")
        expect(key.string).to eq('SSH KEY')
      end
    end

    describe 'with GITLAB_SHELL_KNOWN_HOSTS' do
      let(:gl_projects) { build_gitlab_projects('fetch-remote', repos_path, project_name, remote_name, '600') }
      let(:cmd) { %W(git --git-dir=#{full_path} fetch #{remote_name} --prune --quiet --tags) }

      around(:each) do |example|
        stub_env('GITLAB_SHELL_KNOWN_HOSTS' => 'KNOWN HOSTS') { example.run }
      end

      it 'sets GIT_SSH to a custom script' do
        script = stub_tempfile('scriptFile', 'gitlab-shell-ssh-wrapper', chmod: 0o755)
        key = stub_tempfile('/tmp files/knownHosts', 'gitlab-shell-known-hosts', chmod: 0o400)

        stub_spawn({ 'GIT_SSH' => 'scriptFile' }, *cmd)

        expect(gl_projects.exec).to be true

        expect(script.string).to eq("#!/bin/sh\nexec ssh '-oStrictHostKeyChecking=\"yes\"' '-oUserKnownHostsFile=\"/tmp files/knownHosts\"' \"$@\"")
        expect(key.string).to eq('KNOWN HOSTS')
      end
    end
  end

  describe :import_project do
    context 'success import' do
      let(:gl_projects) { build_gitlab_projects('import-project', tmp_repos_path, repo_name, 'https://github.com/randx/six.git') }

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
      let(:gl_projects) { build_gitlab_projects('import-project', tmp_repos_path, repo_name, 'https://github.com/randx/six.git') }

      it 'should import only once' do
        gl_projects.exec.should be_true
        gl_projects.exec.should be_false
      end
    end

    context 'timeout' do
      let(:gl_projects) { build_gitlab_projects('import-project', tmp_repos_path, repo_name, 'https://github.com/gitlabhq/gitlabhq.git', '1') }

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
    let(:gl_projects_fork) { build_gitlab_projects('fork-project', tmp_repos_path, source_repo_name, tmp_repos_path, 'forked-to-namespace') }
    let(:gl_projects_import) { build_gitlab_projects('import-project', tmp_repos_path, source_repo_name, 'https://github.com/randx/six.git') }

    before do
      gl_projects_import.exec
    end

    it "should not fork without a source repository path" do
      missing_arg = build_gitlab_projects('fork-project', tmp_repos_path, source_repo_name)
      $logger.should_receive(:error).with("fork-project failed: no destination repository path provided.")
      missing_arg.exec.should be_false
    end

    it "should not fork without a destination namespace" do
      missing_arg = build_gitlab_projects('fork-project', tmp_repos_path, source_repo_name, tmp_repos_path)
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
      File.exists?(File.join(dest_repo, '/hooks/pre-receive')).should be_true
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

    context 'different storages' do
      let(:alternative_repos_path) { File.join(ROOT_PATH, 'tmp', 'alternative') }
      let(:dest_repo) { File.join(alternative_repos_path, 'forked-to-namespace', repo_name) }
      let(:gl_projects_fork) { build_gitlab_projects('fork-project', tmp_repos_path, source_repo_name, alternative_repos_path, 'forked-to-namespace') }

      before do
        FileUtils.mkdir_p(alternative_repos_path)
      end

      after do
        FileUtils.rm_rf(alternative_repos_path)
      end

      it "should fork the repo" do
        # create destination namespace
        FileUtils.mkdir_p(File.join(alternative_repos_path, 'forked-to-namespace'))
        gl_projects_fork.exec.should be_true
        File.exists?(dest_repo).should be_true
        File.exists?(File.join(dest_repo, '/hooks/pre-receive')).should be_true
        File.exists?(File.join(dest_repo, '/hooks/post-receive')).should be_true
      end
    end
  end

  describe :exec do
    it 'should puts message if unknown command arg' do
      gitlab_projects = build_gitlab_projects('edit-project', tmp_repos_path, repo_name)
      gitlab_projects.should_receive(:puts).with('not allowed')
      gitlab_projects.exec
    end

    it 'should log a warning for unknown commands' do
      gitlab_projects = build_gitlab_projects('hurf-durf', tmp_repos_path, repo_name)
      $logger.should_receive(:warn).with('Attempt to execute invalid gitlab-projects command "hurf-durf".')
      gitlab_projects.exec
    end
  end

  def build_gitlab_projects(*args)
    argv(*args)
    GitlabProjects.new
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
