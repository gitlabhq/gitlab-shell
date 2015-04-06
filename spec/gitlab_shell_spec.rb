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
    GitlabShell.new.tap do |shell|
      shell.stub(exec_cmd: :exec_called)
      shell.stub(api: api)
    end
  end

  let(:api) do
    double(GitlabNet).tap do |api|
      api.stub(discover: { 'name' => 'John Doe' })
      api.stub(check_access: GitAccessStatus.new(true))
    end
  end

  let(:key_id) { "key-#{rand(100) + 100}" }
  let(:tmp_repos_path) { File.join(ROOT_PATH, 'tmp', 'repositories') }

  before do
    GitlabConfig.any_instance.stub(repos_path: tmp_repos_path, audit_usernames: false)
  end

  describe :initialize do
    before { ssh_cmd 'git-receive-pack' }

    its(:key_id) { should == key_id }
    its(:repos_path) { should == tmp_repos_path }
  end

  describe :parse_cmd do
    describe 'git' do
      context 'w/o namespace' do
        before do
          ssh_cmd 'git-upload-pack gitlab-ci.git'
          subject.send :parse_cmd
        end

        its(:repo_name) { should == 'gitlab-ci.git' }
        its(:git_cmd) { should == 'git-upload-pack' }
      end

      context 'namespace' do
        before do
          ssh_cmd 'git-upload-pack dmitriy.zaporozhets/gitlab-ci.git'
          subject.send :parse_cmd
        end

        its(:repo_name) { should == 'dmitriy.zaporozhets/gitlab-ci.git' }
        its(:git_cmd) { should == 'git-upload-pack' }
      end

      context 'with an invalid number of arguments' do
        before { ssh_cmd 'foobar' }

        it "should raise an DisallowedCommandError" do
          expect { subject.send :parse_cmd }.to raise_error(GitlabShell::DisallowedCommandError)
        end
      end
    end

    describe 'git-annex' do
      let(:repo_path) { File.join(tmp_repos_path, 'dzaporozhets/gitlab.git') }

      before do
        GitlabConfig.any_instance.stub(git_annex_enabled?: true)

        # Create existing project
        FileUtils.mkdir_p(repo_path)
        cmd = %W(git --git-dir=#{repo_path} init --bare)
        system(*cmd)

        ssh_cmd 'git-annex-shell inannex /~/dzaporozhets/gitlab.git SHA256E'
        subject.send :parse_cmd
      end

      its(:repo_name) { should == 'dzaporozhets/gitlab.git' }
      its(:git_cmd) { should == 'git-annex-shell' }

      it 'should init git-annex' do
        File.exists?(File.join(tmp_repos_path, 'dzaporozhets/gitlab.git/annex')).should be_true
      end
    end
  end

  describe :exec do
    context 'git-upload-pack' do
      before { ssh_cmd 'git-upload-pack gitlab-ci.git' }
      after { subject.exec }

      it "should process the command" do
        subject.should_receive(:process_cmd).with()
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with("git-upload-pack", File.join(tmp_repos_path, 'gitlab-ci.git'))
      end

      it "should set the GL_ID environment variable" do
        ENV.should_receive("[]=").with("GL_ID", key_id)
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<git-upload-pack #{File.join(tmp_repos_path, 'gitlab-ci.git')}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end

      it "should use usernames if configured to do so" do
        GitlabConfig.any_instance.stub(audit_usernames: true)
        $logger.should_receive(:info) { |msg| msg.should =~ /for John Doe/ }
      end
    end

    context 'git-receive-pack' do
      before { ssh_cmd 'git-receive-pack gitlab-ci.git' }
      after { subject.exec }

      it "should process the command" do
        subject.should_receive(:process_cmd).with()
      end

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with("git-receive-pack", File.join(tmp_repos_path, 'gitlab-ci.git'))
      end

      it "should log the command execution" do
        message = "gitlab-shell: executing git command "
        message << "<git-receive-pack #{File.join(tmp_repos_path, 'gitlab-ci.git')}> "
        message << "for user with key #{key_id}."
        $logger.should_receive(:info).with(message)
      end
    end

    context 'arbitrary command' do
      before { ssh_cmd 'arbitrary command' }
      after { subject.exec }

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
      before { ssh_cmd nil }
      after { subject.exec }

      it "should call api.discover" do
        api.should_receive(:discover).with(key_id)
      end
    end

    context "failed connection" do
      before {
        ssh_cmd 'git-upload-pack gitlab-ci.git'
        api.stub(:check_access).and_raise(GitlabNet::ApiUnreachableError)
      }
      after { subject.exec }

      it "should not process the command" do
        subject.should_not_receive(:process_cmd)
      end

      it "should not execute the command" do
        subject.should_not_receive(:exec_cmd)
      end
    end

    describe 'git-annex' do
      before do
        GitlabConfig.any_instance.stub(git_annex_enabled?: true)
        ssh_cmd 'git-annex-shell commit /~/gitlab-ci.git SHA256'
      end

      after { subject.exec }

      it "should execute the command" do
        subject.should_receive(:exec_cmd).with("git-annex-shell", "commit", File.join(tmp_repos_path, 'gitlab-ci.git'), "SHA256")
      end
    end
  end

  describe :validate_access do
    before { ssh_cmd 'git-upload-pack gitlab-ci.git' }
    after { subject.exec }

    it "should call api.check_access" do
      api.should_receive(:check_access).
        with('git-upload-pack', 'gitlab-ci.git', key_id, '_any')
    end

    it "should disallow access and log the attempt if check_access returns false status" do
      api.stub(check_access: GitAccessStatus.new(false))
      message = "gitlab-shell: Access denied for git command <git-upload-pack gitlab-ci.git> "
      message << "by user with key #{key_id}."
      $logger.should_receive(:warn).with(message)
    end
  end

  describe :exec_cmd do
    let(:shell) { GitlabShell.new }
    before { Kernel.stub!(:exec) }

    it "uses Kernel::exec method" do
      Kernel.should_receive(:exec).with(kind_of(Hash), 1, unsetenv_others: true).once
      shell.send :exec_cmd, 1
    end
  end

  describe :api do
    let(:shell) { GitlabShell.new }
    subject { shell.send :api }

    it { should be_a(GitlabNet) }
  end

  describe :escape_path do
    let(:shell) { GitlabShell.new }
    before { File.stub(:absolute_path) { 'y' } }
    subject { -> { shell.send(:escape_path, 'z') } }

    it { should raise_error(GitlabShell::InvalidRepositoryPathError) }
  end

  def ssh_cmd(cmd)
    ENV['SSH_ORIGINAL_COMMAND'] = cmd
  end
end
