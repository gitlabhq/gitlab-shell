require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'

describe GitlabKeys do
  describe :initialize do
    before do
      argv('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E')
      @gl_keys = GitlabKeys.new
    end

    it { @gl_keys.key.should == 'ssh-rsa AAAAB3NzaDAxx2E' }
    it { @gl_keys.instance_variable_get(:@command).should == 'add-key' }
    it { @gl_keys.instance_variable_get(:@key_id).should == 'key-741' }
  end

  describe :add_key do
    before do
      argv('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E')
      @gl_keys = GitlabKeys.new
    end

    it "should receive valid cmd" do
      valid_cmd = "echo 'command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa AAAAB3NzaDAxx2E' >> #{GitlabConfig.new.auth_file}"
      @gl_keys.should_receive(:system).with(valid_cmd)
      @gl_keys.send :add_key
    end
  end

  describe :rm_key do
    before do
      argv('rm-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E')
      @gl_keys = GitlabKeys.new
    end

    it "should receive valid cmd" do
      valid_cmd = "sed -i '/shell key-741/d' #{GitlabConfig.new.auth_file}"
      @gl_keys.should_receive(:system).with(valid_cmd)
      @gl_keys.send :rm_key
    end
  end

  def argv(*args)
    args.each_with_index do |arg, i|
      ARGV[i] = arg
    end
  end
end
