require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'

describe GitlabShell do
  subject do
    ARGV[0] = 'key-56'
    GitlabShell.new.tap do |shell|
      shell.stub(process_cmd: true)
      shell.stub(api: api)
    end
  end
  let(:api) do
    double(GitlabNet).tap do |api|
      api.stub(discover: 'John Doe')
      api.stub(allowed?: true)
    end
  end

  describe :initialize do
    before { ssh_cmd 'git-receive-pack' }

    its(:key_id) { should == 'key-56' }
    its(:repos_path) { should == "/home/git/repositories" }
  end

  describe :parse_cmd do
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
  end

  describe :exec do
    context 'git-upload-pack' do
      before { ssh_cmd 'git-upload-pack gitlab-ci.git' }
      after { subject.exec }

      it "should process the command" do
        subject.should_receive(:process_cmd).with()
      end
    end

    context 'git-receive-pack' do
      before { ssh_cmd 'git-receive-pack gitlab-ci.git' }
      after { subject.exec }

      it "should process the command" do
        subject.should_receive(:process_cmd).with()
      end
    end

    context 'arbitrary command' do
      before { ssh_cmd 'arbitrary command' }
      after { subject.exec }

      it "should not process the command" do
        subject.should_not_receive(:process_cmd)
      end
    end

    context 'no command' do
      before { ssh_cmd nil }
      after { subject.exec }

      it "should call api.discover" do
        api.should_receive(:discover).with('key-56')
      end
    end
  end

  describe :validate_access do
    before { ssh_cmd 'git-upload-pack gitlab-ci.git' }
    after { subject.exec }

    it "should call api.allowed?" do
      api.should_receive(:allowed?).
        with('git-upload-pack', 'gitlab-ci.git', 'key-56', '_any')
    end
  end

  def ssh_cmd(cmd)
    ENV['SSH_ORIGINAL_COMMAND'] = cmd
  end

end
