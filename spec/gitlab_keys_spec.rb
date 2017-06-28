require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'
require 'stringio'

describe GitlabKeys do
  before do
    $logger = double('logger').as_null_object
  end

  describe '.command' do
    it 'returns the "command" part of the key line' do
      command = "#{ROOT_PATH}/bin/gitlab-shell key-123"
      expect(described_class.command('key-123')).to eq(command)
    end

    it 'raises KeyError on invalid input' do
      expect { described_class.command("\nssh-rsa AAA") }.to raise_error(described_class::KeyError)
    end
  end

  describe '.key_line' do
    let(:line) { %(command="#{ROOT_PATH}/bin/gitlab-shell key-741",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa AAAAB3NzaDAxx2E) }

    it 'returns the key line' do
      expect(described_class.key_line('key-741', 'ssh-rsa AAAAB3NzaDAxx2E')).to eq(line)
    end

    it 'silently removes a trailing newline' do
      expect(described_class.key_line('key-741', "ssh-rsa AAAAB3NzaDAxx2E\n")).to eq(line)
    end

    it 'raises KeyError on invalid input' do
      expect { described_class.key_line('key-741', "ssh-rsa AAA\nssh-rsa AAA") }.to raise_error(described_class::KeyError)
    end
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
      before { allow(gitlab_keys).to receive(:open) }
      before { create_authorized_keys_fixture }

      it "should log an add-key event" do
        $logger.should_receive(:info).with('Adding key key-741 => "ssh-rsa AAAAB3NzaDAxx2E"')
        gitlab_keys.send :add_key
      end

      it "should return true" do
        gitlab_keys.send(:add_key).should be_true
      end
    end
  end

  describe :list_keys do
    let(:gitlab_keys) do
      build_gitlab_keys('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E')
    end

    it 'adds a key and lists it' do
      create_authorized_keys_fixture
      gitlab_keys.send :add_key
      auth_line1 = 'key-741 AAAAB3NzaDAxx2E'
      gitlab_keys.send(:list_keys).should == "#{auth_line1}\n"
    end
  end

  describe :list_key_ids do
    let(:gitlab_keys) { build_gitlab_keys('list-key-ids') }
    before do
      create_authorized_keys_fixture(
        existing_content:
          "key-1\tssh-dsa AAA\nkey-2\tssh-rsa BBB\nkey-3\tssh-rsa CCC\nkey-9000\tssh-rsa DDD\n"
      )
    end

    it 'outputs the key IDs, separated by newlines' do
      output = capture_stdout do
        gitlab_keys.send(:list_key_ids)
      end
      output.should match "1\n2\n3\n9000"
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
        gitlab_keys.should_receive(:open).and_yield(double(:file, puts: nil, chmod: nil))
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

  describe :stdin do
    let(:gitlab_keys) { build_gitlab_keys }
    subject { gitlab_keys.send :stdin }
    before { $stdin = 1 }

    it { should equal(1) }
  end

  describe :rm_key do
    let(:gitlab_keys) { build_gitlab_keys('rm-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it "removes the right line" do
      create_authorized_keys_fixture
      other_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-742\",options ssh-rsa AAAAB3NzaDAxx2E"
      delete_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",options ssh-rsa AAAAB3NzaDAxx2E"
      open(tmp_authorized_keys_path, 'a') do |auth_file|
        auth_file.puts delete_line
        auth_file.puts other_line
      end
      gitlab_keys.send :rm_key
      erased_line = delete_line.gsub(/./, '#')
      File.read(tmp_authorized_keys_path).should == "existing content\n#{erased_line}\n#{other_line}\n"
    end

    context "without file writing" do
      before do
        gitlab_keys.stub(:open)
        gitlab_keys.stub(:lock).and_yield
      end

      it "should log an rm-key event" do
        $logger.should_receive(:info).with('Removing key key-741')
        gitlab_keys.send :rm_key
      end

      it "should return true" do
        gitlab_keys.send(:rm_key).should be_true
      end
    end

    context 'without key content' do
      let(:gitlab_keys) { build_gitlab_keys('rm-key', 'key-741') }

      it "removes the right line by key ID" do
        create_authorized_keys_fixture
        other_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-742\",options ssh-rsa AAAAB3NzaDAxx2E"
        delete_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",options ssh-rsa AAAAB3NzaDAxx2E"
        open(tmp_authorized_keys_path, 'a') do |auth_file|
          auth_file.puts delete_line
          auth_file.puts other_line
        end
        gitlab_keys.send :rm_key
        erased_line = delete_line.gsub(/./, '#')
        File.read(tmp_authorized_keys_path).should == "existing content\n#{erased_line}\n#{other_line}\n"
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

  describe :check_permissions do
    let(:gitlab_keys) { build_gitlab_keys('check-permissions') }

    it 'returns true when the file can be opened' do
      create_authorized_keys_fixture
      expect(gitlab_keys.exec).to eq(true)
    end

    it 'returns false if opening raises an exception' do
      gitlab_keys.should_receive(:open_auth_file).and_raise("imaginary error")
      expect(gitlab_keys.exec).to eq(false)
    end

    it 'creates the keys file if it does not exist' do
      create_authorized_keys_fixture
      FileUtils.rm(tmp_authorized_keys_path)
      expect(gitlab_keys.exec).to eq(true)
      expect(File.exist?(tmp_authorized_keys_path)).to eq(true)
    end
  end

  describe :exec do
    it 'add-key arg should execute add_key method' do
      gitlab_keys = build_gitlab_keys('add-key')
      gitlab_keys.should_receive(:add_key)
      gitlab_keys.exec
    end

    it 'batch-add-keys arg should execute batch_add_keys method' do
      gitlab_keys = build_gitlab_keys('batch-add-keys')
      gitlab_keys.should_receive(:batch_add_keys)
      gitlab_keys.exec
    end

    it 'rm-key arg should execute rm_key method' do
      gitlab_keys = build_gitlab_keys('rm-key')
      gitlab_keys.should_receive(:rm_key)
      gitlab_keys.exec
    end

    it 'clear arg should execute clear method' do
      gitlab_keys = build_gitlab_keys('clear')
      gitlab_keys.should_receive(:clear)
      gitlab_keys.exec
    end

    it 'check-permissions arg should execute check_permissions method' do
      gitlab_keys = build_gitlab_keys('check-permissions')
      gitlab_keys.should_receive(:check_permissions)
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
    before do
      GitlabKeys.any_instance.stub(lock_file: tmp_lock_file_path)
    end

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
      ARGV[i] = arg.freeze
    end
  end

  def create_authorized_keys_fixture(existing_content: 'existing content')
    FileUtils.mkdir_p(File.dirname(tmp_authorized_keys_path))
    open(tmp_authorized_keys_path, 'w') { |file| file.puts(existing_content) }
    gitlab_keys.stub(auth_file: tmp_authorized_keys_path)
  end

  def tmp_authorized_keys_path
    File.join(ROOT_PATH, 'tmp', 'authorized_keys')
  end

  def tmp_lock_file_path
    tmp_authorized_keys_path + '.lock'
  end

  def capture_stdout(&blk)
    old = $stdout
    $stdout = fake = StringIO.new
    blk.call
    fake.string
  ensure
    $stdout = old
  end
end
