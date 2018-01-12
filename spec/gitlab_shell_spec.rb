require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'
require_relative '../lib/gitlab_access_status'

describe GitlabShell do
  before do
    FileUtils.mkdir_p(tmp_repos_path)
  end

  after do
    FileUtils.rm_rf(tmp_repos_path)
  end

  subject do
    ARGV[0] = key_id
    GitlabShell.new(key_id).tap do |shell|
      shell.stub(exec_cmd: :exec_called)
      shell.stub(api: api)
    end
  end

  let(:gitaly_check_access) { GitAccessStatus.new(
    true,
    'ok',
    gl_repository: gl_repository,
    gl_username: gl_username,
    repository_path: repo_path,
    gitaly: { 'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default'} , 'address' => 'unix:gitaly.socket' }
  )
  }

  let(:api) do
    double(GitlabNet).tap do |api|
      api.stub(discover: { 'name' => 'John Doe' })
      api.stub(check_access: GitAccessStatus.new(
                true,
                'ok',
                gl_repository: gl_repository,
                gl_username: gl_username,
                repository_path: repo_path,
                gitaly: nil))
      api.stub(two_factor_recovery_codes: {
        'success' => true,
        'recovery_codes' => ['f67c514de60c4953', '41278385fc00c1e0']
      })
    end
  end

  let(:key_id) { "key-#{rand(100) + 100}" }
  let(:ssh_cmd) { nil }
  let(:tmp_repos_path) { File.join(ROOT_PATH, 'tmp', 'repositories') }

  let(:repo_name) { 'gitlab-ci.git' }
  let(:repo_path) { File.join(tmp_repos_path, repo_name) }
  let(:gl_repository) { 'project-1' }
  let(:gl_username) { 'testuser' }

  before do
    GitlabConfig.any_instance.stub(audit_usernames: false)
  end

  describe :initialize do
    let(:ssh_cmd) { 'git-receive-pack' }

    its(:key_id) { should == key_id }
  end

  describe :parse_cmd do
    describe 'git' do
      context 'w/o namespace' do
        let(:ssh_args) { %W(git-upload-pack gitlab-ci.git) }

        before do
          subject.send :parse_cmd, ssh_args
        end

        its(:repo_name) { should == 'gitlab-ci.git' }
        its(:command) { should == 'git-upload-pack' }
      end

      context 'namespace' do
        let(:repo_name) { 'dmitriy.zaporozhets/gitlab-ci.git' }
        let(:ssh_args) { %W(git-upload-pack dmitriy.zaporozhets/gitlab-ci.git) }

        before do
          subject.send :parse_cmd, ssh_args
        end

        its(:repo_name) { should == 'dmitriy.zaporozhets/gitlab-ci.git' }
        its(:command) { should == 'git-upload-pack' }
      end

      context 'with an invalid number of arguments' do
        let(:ssh_args) { %W(foobar) }

        it "should raise an DisallowedCommandError" do
          expect { subject.send :parse_cmd, ssh_args }.to raise_error(GitlabShell::DisallowedCommandError)
        end
      end

      context 'with an API command' do
        before do
          subject.send :parse_cmd, ssh_args
        end

        context 'when generating recovery codes' do
          let(:ssh_args) { %w(2fa_recovery_codes) }

          it 'sets the correct command' do
            expect(subject.command).to eq('2fa_recovery_codes')
          end

          it 'does not set repo name' do
            expect(subject.repo_name).to be_nil
          end
        end
      end
    end

    describe 'git-lfs' do
      let(:repo_name) { 'dzaporozhets/gitlab.git' }
      let(:ssh_args) { %W(git-lfs-authenticate dzaporozhets/gitlab.git download) }

      before do
        subject.send :parse_cmd, ssh_args
      end

      its(:repo_name) { should == 'dzaporozhets/gitlab.git' }
      its(:command) { should == 'git-lfs-authenticate' }
      its(:git_access) { should == 'git-upload-pack' }
    end

    describe 'git-lfs old clients' do
      let(:repo_name) { 'dzaporozhets/gitlab.git' }
      let(:ssh_args) { %W(git-lfs-authenticate dzaporozhets/gitlab.git download long_oid) }

      before do
        subject.send :parse_cmd, ssh_args
      end

      its(:repo_name) { should == 'dzaporozhets/gitlab.git' }
      its(:command) { should == 'git-lfs-authenticate' }
      its(:git_access) { should == 'git-upload-pack' }
    end
  end

  describe :exec do
    let(:gitaly_message) { JSON.dump({ 'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default' }, 'gl_repository' => gl_repository, 'gl_id' => key_id, 'gl_username' => gl_username}) }

    shared_examples_for 'upload-pack' do |command|
      let(:ssh_cmd) { "#{command} gitlab-ci.git" }
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        subject.should_receive(:process_cmd).with(%W(git-upload-pack gitlab-ci.git))
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with('git-upload-pack', repo_path)
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<git-upload-pack #{repo_path}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end

      it "should use usernames if configured to do so" do
        GitlabConfig.any_instance.stub(audit_usernames: true)
        $logger.should_receive(:info).with(/for John Doe/)
      end
    end

    context 'git-upload-pack' do
      it_behaves_like 'upload-pack', 'git-upload-pack'
    end

    context 'git upload-pack' do
      it_behaves_like 'upload-pack', 'git upload-pack'
    end

    context 'gitaly-upload-pack' do
      let(:ssh_cmd) { "git-upload-pack gitlab-ci.git" }
      before {
        api.stub(check_access: gitaly_check_access)
      }
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        subject.should_receive(:process_cmd).with(%W(git-upload-pack gitlab-ci.git))
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with(File.join(ROOT_PATH, "bin/gitaly-upload-pack"), 'unix:gitaly.socket', gitaly_message)
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<gitaly-upload-pack unix:gitaly.socket #{gitaly_message}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end

      it "should use usernames if configured to do so" do
        GitlabConfig.any_instance.stub(audit_usernames: true)
        $logger.should_receive(:info) { |msg| msg.should =~ /for John Doe/ }
      end
    end

    context 'git-receive-pack' do
      let(:ssh_cmd) { "git-receive-pack gitlab-ci.git" }
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        subject.should_receive(:process_cmd).with(%W(git-receive-pack gitlab-ci.git))
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with('git-receive-pack', repo_path)
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<git-receive-pack #{repo_path}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end
    end

    context 'gitaly-receive-pack' do
      let(:ssh_cmd) { "git-receive-pack gitlab-ci.git" }
      before {
        api.stub(check_access: gitaly_check_access)
      }
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        subject.should_receive(:process_cmd).with(%W(git-receive-pack gitlab-ci.git))
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with(File.join(ROOT_PATH, "bin/gitaly-receive-pack"), 'unix:gitaly.socket', gitaly_message)
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<gitaly-receive-pack unix:gitaly.socket #{gitaly_message}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end

      it "should use usernames if configured to do so" do
        GitlabConfig.any_instance.stub(audit_usernames: true)
        $logger.should_receive(:info).with(/for John Doe/)
      end
    end

    context 'arbitrary command' do
      let(:ssh_cmd) { 'arbitrary command' }
      after { subject.exec(ssh_cmd) }

      it "should not process the command" do
        subject.should_not_receive(:process_cmd)
      end

      it "should not execute the command" do
        subject.should_not_receive(:exec_cmd)
      end

      it "should log the attempt" do
        message = "gitlab-shell: Attempt to execute disallowed command <arbitrary command> by user with key #{key_id}."
        $logger.should_receive(:warn).with(message)
      end
    end

    context 'no command' do
      after { subject.exec(nil) }

      it "should call api.discover" do
        api.should_receive(:discover).with(key_id)
      end
    end

    context "failed connection" do
      let(:ssh_cmd) { 'git-upload-pack gitlab-ci.git' }

      before {
        api.stub(:check_access).and_raise(GitlabNet::ApiUnreachableError)
      }
      after { subject.exec(ssh_cmd) }

      it "should not process the command" do
        subject.should_not_receive(:process_cmd)
      end

      it "should not execute the command" do
        subject.should_not_receive(:exec_cmd)
      end
    end

    context 'with an API command' do
      before do
        allow(subject).to receive(:continue?).and_return(true)
      end

      context 'when generating recovery codes' do
        let(:ssh_cmd) { '2fa_recovery_codes' }
        after do
          subject.exec(ssh_cmd)
        end

        it 'does not call verify_access' do
          expect(subject).not_to receive(:verify_access)
        end

        it 'calls the corresponding method' do
          expect(subject).to receive(:api_2fa_recovery_codes)
        end

        it 'outputs recovery codes' do
          expect($stdout).to receive(:puts)
            .with(/f67c514de60c4953\n41278385fc00c1e0/)
        end

        context 'when the process is unsuccessful' do
          it 'displays the error to the user' do
            api.stub(two_factor_recovery_codes: {
              'success' => false,
              'message' => 'Could not find the given key'
            })

            expect($stdout).to receive(:puts)
              .with(/Could not find the given key/)
          end
        end
      end
    end
  end

  describe :validate_access do
    let(:ssh_cmd) { "git-upload-pack gitlab-ci.git" }

    describe 'check access with api' do
      after { subject.exec(ssh_cmd) }

      it "should call api.check_access" do
        api.should_receive(:check_access).with('git-upload-pack', nil, 'gitlab-ci.git', key_id, '_any', 'ssh')
      end

      it "should disallow access and log the attempt if check_access returns false status" do
        api.stub(check_access: GitAccessStatus.new(
                  false,
                  'denied',
                  gl_repository: nil,
                  gl_username: nil,
                  repository_path: nil,
                  gitaly: nil))
        message = "gitlab-shell: Access denied for git command <git-upload-pack gitlab-ci.git> "
        message << "by user with key #{key_id}."
        $logger.should_receive(:warn).with(message)
      end
    end

    describe 'set the repository path' do
      context 'with a correct path' do
        before { subject.exec(ssh_cmd) }

        its(:repo_path) { should == repo_path }
      end

      context "with a path that doesn't match an absolute path" do
        before do
          File.stub(:absolute_path) { 'y/gitlab-ci.git' }
        end

        it "refuses to assign the path" do
          $stderr.should_receive(:puts).with("GitLab: Invalid repository path")
          expect(subject.exec(ssh_cmd)).to be_false
        end
      end
    end
  end

  describe :exec_cmd do
    let(:shell) { GitlabShell.new(key_id) }
    let(:env) do
      {
        'HOME' => ENV['HOME'],
        'PATH' => ENV['PATH'],
        'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'],
        'LANG' => ENV['LANG'],
        'GL_ID' => key_id,
        'GL_PROTOCOL' => 'ssh',
        'GL_REPOSITORY' => gl_repository,
        'GL_USERNAME' => 'testuser'
      }
    end
    let(:exec_options) { { unsetenv_others: true, chdir: ROOT_PATH } }
    before do
      Kernel.stub(:exec)
      shell.gl_repository = gl_repository
      shell.username = gl_username
    end

    it "uses Kernel::exec method" do
      Kernel.should_receive(:exec).with(env, 1, 2, exec_options).once
      shell.send :exec_cmd, 1, 2
    end

    it "refuses to execute a lone non-array argument" do
      expect { shell.send :exec_cmd, 1 }.to raise_error(GitlabShell::DisallowedCommandError)
    end

    it "allows one argument if it is an array" do
      Kernel.should_receive(:exec).with(env, [1, 2], exec_options).once
      shell.send :exec_cmd, [1, 2]
    end

    context "when specifying a git_tracing log file" do
      let(:git_trace_log_file) { '/tmp/git_trace_performance.log' }

      before do
        GitlabConfig.any_instance.stub(git_trace_log_file: git_trace_log_file)
        shell
      end

      it "uses GIT_TRACE_PERFORMANCE" do
        expected_hash = hash_including(
          'GIT_TRACE' => git_trace_log_file,
          'GIT_TRACE_PACKET' => git_trace_log_file,
          'GIT_TRACE_PERFORMANCE' => git_trace_log_file
        )
        Kernel.should_receive(:exec).with(expected_hash, [1, 2], exec_options).once

        shell.send :exec_cmd, [1, 2]
      end

      context "when provides a relative path" do
        let(:git_trace_log_file) { 'git_trace_performance.log' }

        it "does not uses GIT_TRACE*" do
          # If we try to use it we'll show a warning to the users
          expected_hash = hash_excluding(
            'GIT_TRACE', 'GIT_TRACE_PACKET', 'GIT_TRACE_PERFORMANCE'
          )
          Kernel.should_receive(:exec).with(expected_hash, [1, 2], exec_options).once

          shell.send :exec_cmd, [1, 2]
        end

        it "writes an entry on the log" do
          expect($logger).to receive(:warn).
            with("gitlab-shell: is configured to trace git commands with #{git_trace_log_file.inspect} but an absolute path needs to be provided")

          Kernel.should_receive(:exec).with(env, [1, 2], exec_options).once
          shell.send :exec_cmd, [1, 2]
        end
      end

      context "when provides a file not writable" do
        before do
          expect(File).to receive(:open).with(git_trace_log_file, 'a').and_raise(Errno::EACCES)
        end

        it "does not uses GIT_TRACE*" do
          # If we try to use it we'll show a warning to the users
          expected_hash = hash_excluding(
            'GIT_TRACE', 'GIT_TRACE_PACKET', 'GIT_TRACE_PERFORMANCE'
          )
          Kernel.should_receive(:exec).with(expected_hash, [1, 2], exec_options).once

          shell.send :exec_cmd, [1, 2]
        end

        it "writes an entry on the log" do
          expect($logger).to receive(:warn).
            with("gitlab-shell: is configured to trace git commands with #{git_trace_log_file.inspect} but it's not possible to write in that path Permission denied")

          Kernel.should_receive(:exec).with(env, [1, 2], exec_options).once
          shell.send :exec_cmd, [1, 2]
        end
      end
    end
  end

  describe :api do
    let(:shell) { GitlabShell.new(key_id) }
    subject { shell.send :api }

    it { should be_a(GitlabNet) }
  end
end
