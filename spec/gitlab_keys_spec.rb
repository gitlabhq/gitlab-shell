require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'
require 'stringio'

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

    it "adds a line at the end of the file" do
      create_authorized_keys_fixture
      gitlab_keys.send :add_key
      auth_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa AAAAB3NzaDAxx2E"
      File.read(tmp_authorized_keys_path).should == "existing content\n#{auth_line}\n"
    end

    context "without file writing" do
      before { gitlab_keys.stub(:open) }

      it "should log an add-key event" do
        $logger.should_receive(:info).with('Adding key key-741 => "ssh-rsa AAAAB3NzaDAxx2E"')
        gitlab_keys.send :add_key
      end

      it "should return true" do
        gitlab_keys.send(:add_key).should be_true
      end
    end
  end

  describe :batch_add_keys do
    let(:gitlab_keys) { build_gitlab_keys('batch-add-keys') }
    let(:fake_stdin) { StringIO.new("key-12\tssh-dsa ASDFASGADG\nkey-123\tssh-rsa GFDGDFSGSDFG\n", 'r') }
    before do
      create_authorized_keys_fixture
      gitlab_keys.stub(stdin: fake_stdin)
    end

    it "adds lines at the end of the file" do
      gitlab_keys.send :batch_add_keys
      auth_line1 = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-12\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-dsa ASDFASGADG"
      auth_line2 = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-123\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa GFDGDFSGSDFG"
      File.read(tmp_authorized_keys_path).should == "existing content\n#{auth_line1}\n#{auth_line2}\n"
    end

    context "with invalid input" do
      let(:fake_stdin) { StringIO.new("key-12\tssh-dsa ASDFASGADG\nkey-123\tssh-rsa GFDGDFSGSDFG\nfoo\tbar\tbaz\n", 'r') }

      it "aborts" do
        gitlab_keys.should_receive(:abort)
        gitlab_keys.send :batch_add_keys
      end
    end

    context "without file writing" do
      before do
        gitlab_keys.should_receive(:open).and_yield(mock(:file, puts: nil))
      end

      it "should log an add-key event" do
        $logger.should_receive(:info).with('Adding key key-12 => "ssh-dsa ASDFASGADG"')
        $logger.should_receive(:info).with('Adding key key-123 => "ssh-rsa GFDGDFSGSDFG"')
        gitlab_keys.send :batch_add_keys
      end

      it "should return true" do
        gitlab_keys.send(:batch_add_keys).should be_true
      end
    end
  end

  describe :rm_key do
    let(:gitlab_keys) { build_gitlab_keys('rm-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it "removes the right line" do
      create_authorized_keys_fixture
      other_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-742\",options ssh-rsa AAAAB3NzaDAxx2E"
      open(tmp_authorized_keys_path, 'a') do |auth_file|
        auth_file.puts "command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",options ssh-rsa AAAAB3NzaDAxx2E"
        auth_file.puts other_line
      end
      gitlab_keys.send :rm_key
      File.read(tmp_authorized_keys_path).should == "existing content\n#{other_line}\n"
    end

    context "without file writing" do
      before { Tempfile.stub(:open) }

      it "should log an rm-key event" do
        $logger.should_receive(:info).with('Removing key key-741')
        gitlab_keys.send :rm_key
      end

      it "should return true" do
        gitlab_keys.send(:rm_key).should be_true
      end
    end
  end

  describe :clear do
    let(:gitlab_keys) { build_gitlab_keys('clear') }

    it "should return true" do
      gitlab_keys.stub(:open)
      gitlab_keys.send(:clear).should be_true
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

  describe :lock do
    it "should raise exception if operation lasts more then timeout" do
      key = GitlabKeys.new
      expect do
        key.send :lock, 1 do
          sleep 2
        end
      end.to raise_error
    end

    it "should actually lock file" do
      $global = ""
      key = GitlabKeys.new

      thr1 = Thread.new do
        key.send :lock do
          # Put bigger sleep here to test if main thread will
          # wait for lock file released before executing code
          sleep 1
          $global << "foo"
        end
      end

      # make sure main thread start lock command after
      # thread above
      sleep 0.5

      key.send :lock do
        $global << "bar"
      end

      thr1.join
      $global.should == "foobar"
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

  def create_authorized_keys_fixture
    FileUtils.mkdir_p(File.dirname(tmp_authorized_keys_path))
    open(tmp_authorized_keys_path, 'w') { |file| file.puts('existing content') }
    gitlab_keys.stub(auth_file: tmp_authorized_keys_path)
  end

  def tmp_authorized_keys_path
    File.join(ROOT_PATH, 'tmp', 'authorized_keys')
  end
end
