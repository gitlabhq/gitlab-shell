require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'

describe GitlabShell do
  describe :initialize do
    before do
      ssh_cmd 'git-receive-pack'
      ARGV[0] = 'key-56'
      @shell = GitlabShell.new
    end

    it { @shell.key_id.should == 'key-56' }
    it { @shell.repos_path.should == "/home/git/repositories" }
  end

  describe :parse_cmd do
    context 'w/o namespace' do
      before do
        ssh_cmd 'git-upload-pack gitlab-ci.git'
        @shell = GitlabShell.new
        @shell.send :parse_cmd
      end

      it { @shell.repo_name.should == 'gitlab-ci.git' }
      it { @shell.git_cmd.should == 'git-upload-pack' }
    end

    context 'namespace' do
      before do
        ssh_cmd 'git-upload-pack dmitriy.zaporozhets/gitlab-ci.git'
        @shell = GitlabShell.new
        @shell.send :parse_cmd
      end

      it { @shell.repo_name.should == 'dmitriy.zaporozhets/gitlab-ci.git' }
      it { @shell.git_cmd.should == 'git-upload-pack' }
    end
  end

  describe :exec do
    context 'git-upload-pack' do
      before do
        ssh_cmd 'git-upload-pack gitlab-ci.git'
        stubbed_shell
      end

      it { @shell.exec.should be_true }
    end

    context 'git-receive-pack' do
      before do
        ssh_cmd 'git-receive-pack gitlab-ci.git'
        stubbed_shell
      end

      it { @shell.exec.should be_true }
    end

    context "running a non-git command" do
      before { ssh_cmd 'id' }

      context "without admin access" do
        before { stubbed_shell(false) }
        after { @shell.exec }

        it { @shell.should_not_receive(:process_cmd) }
        it { @shell.should_not_receive(:process_admin_cmd) }
        it { @shell.should_not_receive(:exec_cmd) }
      end

      context "with admin access" do
        before { stubbed_shell(true) }
        after { @shell.exec }

        it { @shell.should_not_receive(:process_cmd) }
        it { @shell.should_receive(:process_admin_cmd) }
        it { @shell.should_receive(:exec_cmd) }
      end
    end

    context "running a shell (no SSH_ORIGINAL_COMMAND)" do
      before { ssh_cmd nil }

      context "without admin access" do
        before { stubbed_shell(false) }
        after { @shell.exec }

        it { @shell.should_not_receive(:process_cmd) }
        it { @shell.should_not_receive(:process_admin_cmd) }
        it { @shell.should_not_receive(:exec_cmd) }
      end

      context "with admin access" do
        before { stubbed_shell(true) }
        after { @shell.exec }

        it { @shell.should_not_receive(:process_cmd) }
        it { @shell.should_receive(:process_admin_cmd) }
        it { @shell.should_receive(:exec_cmd) }
      end
    end
  end

  def ssh_cmd(cmd)
    ENV['SSH_ORIGINAL_COMMAND'] = cmd
  end

  def stubbed_shell(admin = false)
    ARGV[0] = 'key-56'
    @shell = GitlabShell.new
    @shell.stub(validate_access: true)
    @shell.stub(admin?: admin)
    @shell.stub(:exec_cmd)
    @shell.stub(process_cmd: true)
    api = double(GitlabNet)
    api.stub(discover: {'name' => 'John Doe'})
    @shell.stub(api: api)
  end
end
