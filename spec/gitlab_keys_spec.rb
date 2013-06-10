require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'

describe GitlabKeys do
  before do
    $logger = double('logger').as_null_object
  end

  describe :initialize do
    let(:gitlab_keys) { build_gitlab_keys('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it { gitlab_keys.key.should == 'ssh-rsa AAAAB3NzaDAxx2E' }
    it { gitlab_keys.instance_variable_get(:@command).should == 'add-key' }
    it { gitlab_keys.instance_variable_get(:@key_id).should == 'key-741' }
  end

  describe :add_key do
    let(:gitlab_keys) { build_gitlab_keys('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it "should receive valid cmd" do
      valid_cmd = "echo 'command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa AAAAB3NzaDAxx2E' >> #{GitlabConfig.new.auth_file}"
      gitlab_keys.should_receive(:system).with(valid_cmd)
      gitlab_keys.send :add_key
    end

    it "should log an add-key event" do
      $logger.should_receive(:info).with('Adding key key-741 => "ssh-rsa AAAAB3NzaDAxx2E"')
      gitlab_keys.send :add_key
    end
  end

  describe :rm_key do
    let(:gitlab_keys) { build_gitlab_keys('rm-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it "should receive valid cmd" do
      valid_cmd = "sed -i '/shell key-741\"/d' #{GitlabConfig.new.auth_file}"
      gitlab_keys.should_receive(:system).with(valid_cmd)
      gitlab_keys.send :rm_key
    end

    it "should log an rm-key event" do
      $logger.should_receive(:info).with('Removing key key-741')
      gitlab_keys.send :rm_key
    end
  end

  describe :exec do
    it 'add-key arg should execute add_key method' do
      gitlab_keys = build_gitlab_keys('add-key')
      gitlab_keys.should_receive(:add_key)
      gitlab_keys.exec
    end

    it 'rm-key arg should execute rm_key method' do
      gitlab_keys = build_gitlab_keys('rm-key')
      gitlab_keys.should_receive(:rm_key)
      gitlab_keys.exec
    end

    it 'should puts message if unknown command arg' do
      gitlab_keys = build_gitlab_keys('change-key')
      gitlab_keys.should_receive(:puts).with('not allowed')
      gitlab_keys.exec
    end

    it 'should log a warning on unknown commands' do
      gitlab_keys = build_gitlab_keys('nooope')
      gitlab_keys.stub(puts: nil)
      $logger.should_receive(:warn).with('Attempt to execute invalid gitlab-keys command "nooope".')
      gitlab_keys.exec
    end
  end

  def build_gitlab_keys(*args)
    argv(*args)
    GitlabKeys.new
  end

  def argv(*args)
    args.each_with_index do |arg, i|
      ARGV[i] = arg
    end
  end
end
