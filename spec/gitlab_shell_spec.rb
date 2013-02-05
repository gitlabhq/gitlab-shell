require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'

describe GitlabShell do

  describe :initialize do
    before do
      ssh_cmd 'git-receive-pack'
      ARGV[0] = 'dzaporozhets'
      @shell = GitlabShell.new
    end

    it { @shell.username.should == 'dzaporozhets' }
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
  end

  def ssh_cmd(cmd)
    ENV['SSH_ORIGINAL_COMMAND'] = cmd
  end

  def stubbed_shell
    ARGV[0] = 'dzaporozhets'
    @shell = GitlabShell.new
    @shell.stub(validate_access: true)
    @shell.stub(process_cmd: true)
  end
end
