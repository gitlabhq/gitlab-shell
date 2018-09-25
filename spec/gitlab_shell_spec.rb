require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'
require_relative '../lib/gitlab_access_status'

describe GitlabShell do
  before do
    $logger = double('logger').as_null_object
    FileUtils.mkdir_p(tmp_repos_path)
  end

  after do
    FileUtils.rm_rf(tmp_repos_path)
  end

  subject do
    ARGV[0] = gl_id
    GitlabShell.new(gl_id).tap do |shell|
      allow(shell).to receive(:exec_cmd).and_return(:exec_called)
      allow(shell).to receive(:api).and_return(api)
    end
  end

  let(:git_config_options) { ['receive.MaxInputSize=10000'] }

  let(:gitaly_check_access) do
    GitAccessStatus.new(
      true,
      '200',
      'ok',
      gl_repository: gl_repository,
      gl_id: gl_id,
      gl_username: gl_username,
      git_config_options: git_config_options,
      gitaly: { 'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default'} , 'address' => 'unix:gitaly.socket' },
      git_protocol: git_protocol
    )
  end

  let(:api) do
    double(GitlabNet).tap do |api|
      allow(api).to receive(:discover).and_return({ 'name' => 'John Doe', 'username' => 'testuser' })
      allow(api).to receive(:check_access).and_return(GitAccessStatus.new(
                true,
                '200',
                'ok',
                gl_repository: gl_repository,
                gl_id: gl_id,
                gl_username: gl_username,
                git_config_options: nil,
                gitaly: nil,
                git_protocol: git_protocol))
      allow(api).to receive(:two_factor_recovery_codes).and_return({
                 'success' => true,
                 'recovery_codes' => %w[f67c514de60c4953 41278385fc00c1e0]
               })
    end
  end

  let(:gl_id) { "key-#{rand(100) + 100}" }
  let(:ssh_cmd) { nil }
  let(:tmp_repos_path) { File.join(ROOT_PATH, 'tmp', 'repositories') }

  let(:repo_name) { 'gitlab-ci.git' }
  let(:gl_repository) { 'project-1' }
  let(:gl_id) { 'user-1' }
  let(:gl_username) { 'testuser' }
  let(:git_config_options) { ['receive.MaxInputSize=10000'] }
  let(:git_protocol) { 'version=2' }

  before do
    allow_any_instance_of(GitlabConfig).to receive(:audit_usernames).and_return(false)
  end

  describe '#initialize' do
    let(:ssh_cmd) { 'git-receive-pack' }

    it { expect(subject.gl_id).to eq gl_id }
  end

  describe '#parse_cmd' do
    describe 'git' do
      context 'w/o namespace' do
        let(:ssh_args) { %w(git-upload-pack gitlab-ci.git) }

        before do
          subject.send :parse_cmd, ssh_args
        end

        it 'has the correct attributes' do
          expect(subject.repo_name).to eq 'gitlab-ci.git'
          expect(subject.command).to eq 'git-upload-pack'
        end
      end

      context 'namespace' do
        let(:repo_name) { 'dmitriy.zaporozhets/gitlab-ci.git' }
        let(:ssh_args) { %w(git-upload-pack dmitriy.zaporozhets/gitlab-ci.git) }

        before do
          subject.send :parse_cmd, ssh_args
        end

        it 'has the correct attributes' do
          expect(subject.repo_name).to eq 'dmitriy.zaporozhets/gitlab-ci.git'
          expect(subject.command).to eq 'git-upload-pack'
        end
      end

      context 'with an invalid number of arguments' do
        let(:ssh_args) { %w(foobar) }

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
      let(:ssh_args) { %w(git-lfs-authenticate dzaporozhets/gitlab.git download) }

      before do
        subject.send :parse_cmd, ssh_args
      end

      it 'has the correct attributes' do
        expect(subject.repo_name).to eq 'dzaporozhets/gitlab.git'
        expect(subject.command).to eq 'git-lfs-authenticate'
        expect(subject.git_access).to eq 'git-upload-pack'
      end
    end

    describe 'git-lfs old clients' do
      let(:repo_name) { 'dzaporozhets/gitlab.git' }
      let(:ssh_args) { %w(git-lfs-authenticate dzaporozhets/gitlab.git download long_oid) }

      before do
        subject.send :parse_cmd, ssh_args
      end

      it 'has the correct attributes' do
        expect(subject.repo_name).to eq 'dzaporozhets/gitlab.git'
        expect(subject.command).to eq 'git-lfs-authenticate'
        expect(subject.git_access).to eq 'git-upload-pack'
      end
    end
  end

  describe '#exec' do
    let(:gitaly_message) do
      JSON.dump(
        'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default' },
        'gl_repository' => gl_repository,
        'gl_id' => gl_id,
        'gl_username' => gl_username,
        'git_config_options' => git_config_options,
        'git_protocol' => git_protocol
      )
    end

    before do
      allow(ENV).to receive(:[]).with('GIT_PROTOCOL').and_return(git_protocol)
    end

    shared_examples_for 'upload-pack' do |command|
      let(:ssh_cmd) { "#{command} gitlab-ci.git" }
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        expect(subject).to receive(:process_cmd).with(%w(git-upload-pack gitlab-ci.git))
      end

      it "should execute the command" do
        expect(subject).to receive(:exec_cmd).with('git-upload-pack')
      end

      it "should log the command execution" do
        message = "executing git command"
        user_string = "user with id #{gl_id}"
        expect($logger).to receive(:info).with(message, command: "git-upload-pack", user: user_string)
      end

      it "should use usernames if configured to do so" do
        allow_any_instance_of(GitlabConfig).to receive(:audit_usernames).and_return(true)
        expect($logger).to receive(:info).with("executing git command", hash_including(user: 'testuser'))
      end
    end

    context 'gitaly-upload-pack' do
      let(:ssh_cmd) { "git-upload-pack gitlab-ci.git" }

      before do
        allow(api).to receive(:check_access).and_return(gitaly_check_access)
      end

      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        expect(subject).to receive(:process_cmd).with(%w(git-upload-pack gitlab-ci.git))
      end

      it "should execute the command" do
        expect(subject).to receive(:exec_cmd).with(File.join(ROOT_PATH, "bin/gitaly-upload-pack"), gitaly_address: 'unix:gitaly.socket', json_args: gitaly_message, token: nil)
      end

      it "should log the command execution" do
        message = "executing git command"
        user_string = "user with id #{gl_id}"

        expect($logger).to receive(:info).with(message, command: "gitaly-upload-pack unix:gitaly.socket #{gitaly_message}", user: user_string)
      end

      it "should use usernames if configured to do so" do
        allow_any_instance_of(GitlabConfig).to receive(:audit_usernames).and_return(true)
        expect($logger).to receive(:info).with("executing git command", hash_including(user: 'testuser'))
      end
    end

    context 'git-receive-pack' do
      let(:ssh_cmd) { "git-receive-pack gitlab-ci.git" }

      before do
        allow(api).to receive(:check_access).and_return(gitaly_check_access)
      end

      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        expect(subject).to receive(:process_cmd).with(%w(git-receive-pack gitlab-ci.git))
      end

      it "should execute the command" do
        expect(subject).to receive(:exec_cmd).with(File.join(ROOT_PATH, "bin/gitaly-receive-pack"), gitaly_address: 'unix:gitaly.socket', json_args: gitaly_message, token: nil)
      end

      it "should log the command execution" do
        message = "executing git command"
        user_string = "user with id #{gl_id}"
        expect($logger).to receive(:info).with(message, command: "gitaly-receive-pack unix:gitaly.socket #{gitaly_message}", user: user_string)
      end

      context 'with a custom action' do
        let(:fake_payload) { { 'api_endpoints' => [ '/fake/api/endpoint' ], 'data' => {} } }
        let(:custom_action_gitlab_access_status) do
          GitAccessStatus.new(
            true,
            '300',
            'Multiple Choices',
            payload: fake_payload
          )
        end
        let(:action_custom) { double(Action::Custom) }

        before do
          allow(api).to receive(:check_access).and_return(custom_action_gitlab_access_status)
        end

        it "should not process the command" do
          expect(subject).to_not receive(:process_cmd).with(%w(git-receive-pack gitlab-ci.git))
          expect(Action::Custom).to receive(:new).with(gl_id, fake_payload).and_return(action_custom)
          expect(action_custom).to receive(:execute)
        end
      end
    end

    context 'gitaly-receive-pack' do
      let(:ssh_cmd) { "git-receive-pack gitlab-ci.git" }
      before do
        allow(api).to receive(:check_access).and_return(gitaly_check_access)
      end
      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        expect(subject).to receive(:process_cmd).with(%w(git-receive-pack gitlab-ci.git))
      end

      it "should execute the command" do
        expect(subject).to receive(:exec_cmd).with(File.join(ROOT_PATH, "bin/gitaly-receive-pack"), gitaly_address: 'unix:gitaly.socket', json_args: gitaly_message, token: nil)
      end

      it "should log the command execution" do
        message = "executing git command"
        user_string = "user with id #{gl_id}"
        expect($logger).to receive(:info).with(message, command: "gitaly-receive-pack unix:gitaly.socket #{gitaly_message}", user: user_string)
      end

      it "should use usernames if configured to do so" do
        allow_any_instance_of(GitlabConfig).to receive(:audit_usernames).and_return(true)
        expect($logger).to receive(:info).with("executing git command", hash_including(user: 'testuser'))
      end
    end

    shared_examples_for 'upload-archive' do |command|
      let(:ssh_cmd) { "#{command} gitlab-ci.git" }
      let(:exec_cmd_log_params) { exec_cmd_params }

      after { subject.exec(ssh_cmd) }

      it "should process the command" do
        expect(subject).to receive(:process_cmd).with(%w(git-upload-archive gitlab-ci.git))
      end

      it "should execute the command" do
        expect(subject).to receive(:exec_cmd).with(*exec_cmd_params)
      end

      it "should log the command execution" do
        message = "executing git command"
        user_string = "user with id #{gl_id}"
        expect($logger).to receive(:info).with(message, command: exec_cmd_log_params.join(' '), user: user_string)
      end

      it "should use usernames if configured to do so" do
        allow_any_instance_of(GitlabConfig).to receive(:audit_usernames).and_return(true)
        expect($logger).to receive(:info).with("executing git command", hash_including(user: 'testuser'))
      end
    end

    context 'gitaly-upload-archive' do
      before do
        allow(api).to receive(:check_access).and_return(gitaly_check_access)
      end

      it_behaves_like 'upload-archive', 'git-upload-archive' do
        let(:gitaly_executable) { "gitaly-upload-archive" }
        let(:exec_cmd_params) do
          [
            File.join(ROOT_PATH, "bin", gitaly_executable),
            { gitaly_address: 'unix:gitaly.socket', json_args: gitaly_message, token: nil }
          ]
        end
        let(:exec_cmd_log_params) do
          [gitaly_executable, 'unix:gitaly.socket', gitaly_message]
        end
      end
    end

    context 'arbitrary command' do
      let(:ssh_cmd) { 'arbitrary command' }
      after { subject.exec(ssh_cmd) }

      it "should not process the command" do
        expect(subject).not_to receive(:process_cmd)
      end

      it "should not execute the command" do
        expect(subject).not_to receive(:exec_cmd)
      end

      it "should log the attempt" do
        message = 'Denied disallowed command'
        user_string = "user with id #{gl_id}"
        expect($logger).to receive(:warn).with(message, command: 'arbitrary command', user: user_string)
      end
    end

    context 'no command' do
      after { subject.exec(nil) }

      it "should call api.discover" do
        expect(api).to receive(:discover).with(gl_id)
      end
    end

    context "failed connection" do
      let(:ssh_cmd) { 'git-upload-pack gitlab-ci.git' }

      before do
        allow(api).to receive(:check_access).and_raise(GitlabNet::ApiUnreachableError)
      end
      after { subject.exec(ssh_cmd) }

      it "should not process the command" do
        expect(subject).not_to receive(:process_cmd)
      end

      it "should not execute the command" do
        expect(subject).not_to receive(:exec_cmd)
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
            allow(api).to receive(:two_factor_recovery_codes).and_return({
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

  describe '#validate_access' do
    let(:ssh_cmd) { "git-upload-pack gitlab-ci.git" }

    describe 'check access with api' do
      before do
        allow(api).to receive(:check_access).and_return(
          GitAccessStatus.new(
            false,
            'denied',
            gl_repository: nil,
            gl_id: nil,
            gl_username: nil,
            git_config_options: nil,
            gitaly: nil,
            git_protocol: nil))
      end

      after { subject.exec(ssh_cmd) }

      it "should call api.check_access" do
        expect(api).to receive(:check_access).with('git-upload-pack', nil, 'gitlab-ci.git', gl_id, '_any', 'ssh')
      end

      it "should disallow access and log the attempt if check_access returns false status" do
        message = 'Access denied'
        user_string = "user with id #{gl_id}"

        expect($logger).to receive(:warn).with(message, command: 'git-upload-pack gitlab-ci.git', user: user_string)
      end
    end
  end

  describe '#api' do
    let(:shell) { GitlabShell.new(gl_id) }
    subject { shell.send :api }

    it { is_expected.to be_a(GitlabNet) }
  end
end
