require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'
require 'stringio'

describe GitlabKeys do
  before do
    $logger = double('logger').as_null_object
    # The default 'auth_file' value from config.yml.example is '/home/git/.ssh/authorized_keys'
    allow(GitlabConfig).to receive_message_chain(:new, :auth_file).and_return('/home/git/.ssh/authorized_keys')
  end

  describe '.command' do
    it 'the internal "command" utility function' do
      command = "#{ROOT_PATH}/bin/gitlab-shell does-not-validate"
      expect(described_class.command('does-not-validate')).to eq(command)
    end

    it 'does not raise a KeyError on invalid input' do
      command = "#{ROOT_PATH}/bin/gitlab-shell foo\nbar\nbaz\n"
      expect(described_class.command("foo\nbar\nbaz\n")).to eq(command)
    end
  end

  describe '.command_key' do
    it 'returns the "command" part of the key line' do
      command = "#{ROOT_PATH}/bin/gitlab-shell key-123"
      expect(described_class.command_key('key-123')).to eq(command)
    end

    it 'raises KeyError on invalid input' do
      expect { described_class.command_key("\nssh-rsa AAA") }.to raise_error(described_class::KeyError)
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

  describe '.principal_line' do
    let(:line) { %(command="#{ROOT_PATH}/bin/gitlab-shell username-someuser",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty sshUsers) }

    it 'returns the key line' do
      expect(described_class.principal_line('username-someuser', 'sshUsers')).to eq(line)
    end

    it 'silently removes a trailing newline' do
      expect(described_class.principal_line('username-someuser', "sshUsers\n")).to eq(line)
    end

    it 'raises KeyError on invalid input' do
      expect { described_class.principal_line('username-someuser', "sshUsers\nloginUsers") }.to raise_error(described_class::KeyError)
    end
  end

  describe :initialize do
    let(:gitlab_keys) { build_gitlab_keys('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it { expect(gitlab_keys.key).to eq('ssh-rsa AAAAB3NzaDAxx2E') }
    it { expect(gitlab_keys.instance_variable_get(:@command)).to eq('add-key') }
    it { expect(gitlab_keys.instance_variable_get(:@key_id)).to eq('key-741') }
  end

  describe :add_key do
    let(:gitlab_keys) { build_gitlab_keys('add-key', 'key-741', 'ssh-rsa AAAAB3NzaDAxx2E') }

    it "adds a line at the end of the file" do
      create_authorized_keys_fixture
      gitlab_keys.send :add_key
      auth_line = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-741\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa AAAAB3NzaDAxx2E"
      expect(File.read(tmp_authorized_keys_path)).to eq("existing content\n#{auth_line}\n")
    end

    context "without file writing" do
      before { allow(gitlab_keys).to receive(:open) }
      before { create_authorized_keys_fixture }

      it "should log an add-key event" do
        expect($logger).to receive(:info).with("Adding key", {:key_id=>"key-741", :public_key=>"ssh-rsa AAAAB3NzaDAxx2E"})
        gitlab_keys.send :add_key
      end

      it "should return true" do
        expect(gitlab_keys.send(:add_key)).to be_truthy
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
      expect(gitlab_keys.send(:list_keys)).to eq("#{auth_line1}\n")
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
      expect { gitlab_keys.send(:list_key_ids) }.to output("1\n2\n3\n9000\n").to_stdout
    end
  end

  describe :batch_add_keys do
    let(:gitlab_keys) { build_gitlab_keys('batch-add-keys') }
    let(:fake_stdin) { StringIO.new("key-12\tssh-dsa ASDFASGADG\nkey-123\tssh-rsa GFDGDFSGSDFG\n", 'r') }
    before do
      create_authorized_keys_fixture
      allow(gitlab_keys).to receive(:stdin).and_return(fake_stdin)
    end

    it "adds lines at the end of the file" do
      gitlab_keys.send :batch_add_keys
      auth_line1 = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-12\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-dsa ASDFASGADG"
      auth_line2 = "command=\"#{ROOT_PATH}/bin/gitlab-shell key-123\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ssh-rsa GFDGDFSGSDFG"
      expect(File.read(tmp_authorized_keys_path)).to eq("existing content\n#{auth_line1}\n#{auth_line2}\n")
    end

    context "with invalid input" do
      let(:fake_stdin) { StringIO.new("key-12\tssh-dsa ASDFASGADG\nkey-123\tssh-rsa GFDGDFSGSDFG\nfoo\tbar\tbaz\n", 'r') }

      it "aborts" do
        expect(gitlab_keys).to receive(:abort)
        gitlab_keys.send :batch_add_keys
      end
    end

    context "without file writing" do
      before do
        file = double(:file, puts: nil, chmod: nil, flock: nil)
        expect(File).to receive(:open).with(tmp_authorized_keys_path + '.lock', 'w+').and_yield(file)
        expect(File).to receive(:open).with(tmp_authorized_keys_path, "a", 0o600).and_yield(file)
      end

      it "should log an add-key event" do
        expect($logger).to receive(:info).with("Adding key", key_id: 'key-12', public_key: "ssh-dsa ASDFASGADG")
        expect($logger).to receive(:info).with("Adding key", key_id: 'key-123', public_key: "ssh-rsa GFDGDFSGSDFG")
        gitlab_keys.send :batch_add_keys
      end

      it "should return true" do
        expect(gitlab_keys.send(:batch_add_keys)).to be_truthy
      end
    end
  end

  describe :stdin do
    let(:gitlab_keys) { build_gitlab_keys }
    subject { gitlab_keys.send :stdin }
    before { $stdin = 1 }

    it { is_expected.to equal(1) }
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
      expect(File.read(tmp_authorized_keys_path)).to eq("existing content\n#{erased_line}\n#{other_line}\n")
    end

    context "without file writing" do
      before do
        allow(File).to receive(:open).with("#{ROOT_PATH}/config.yml", 'r:bom|utf-8').and_call_original
        allow(File).to receive(:open).with('/home/git/.ssh/authorized_keys', 'r+', 384)
        allow(gitlab_keys).to receive(:lock).and_yield
      end

      it "should log an rm-key event" do
        expect($logger).to receive(:info).with("Removing key", key_id: "key-741")
        gitlab_keys.send :rm_key
      end

      it "should return true" do
        expect(gitlab_keys.send(:rm_key)).to be_truthy
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
        expect(File.read(tmp_authorized_keys_path)).to eq("existing content\n#{erased_line}\n#{other_line}\n")
      end
    end
  end

  describe :clear do
    let(:gitlab_keys) { build_gitlab_keys('clear') }

    it "should return true" do
      allow(File).to receive(:open).with("#{ROOT_PATH}/config.yml", 'r:bom|utf-8').and_call_original
      allow(File).to receive(:open).with('/home/git/.ssh/authorized_keys', 'w', 384)
      expect(gitlab_keys.send(:clear)).to be_truthy
    end
  end

  describe :check_permissions do
    let(:gitlab_keys) { build_gitlab_keys('check-permissions') }

    it 'returns true when the file can be opened' do
      create_authorized_keys_fixture
      expect(gitlab_keys.exec).to eq(true)
    end

    it 'returns false if opening raises an exception' do
      expect(gitlab_keys).to receive(:open_auth_file).and_raise("imaginary error")
      expect { expect(gitlab_keys.exec).to eq(false) }.to output(/imaginary error/).to_stdout
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
      expect(gitlab_keys).to receive(:add_key)
      gitlab_keys.exec
    end

    it 'batch-add-keys arg should execute batch_add_keys method' do
      gitlab_keys = build_gitlab_keys('batch-add-keys')
      expect(gitlab_keys).to receive(:batch_add_keys)
      gitlab_keys.exec
    end

    it 'rm-key arg should execute rm_key method' do
      gitlab_keys = build_gitlab_keys('rm-key')
      expect(gitlab_keys).to receive(:rm_key)
      gitlab_keys.exec
    end

    it 'clear arg should execute clear method' do
      gitlab_keys = build_gitlab_keys('clear')
      expect(gitlab_keys).to receive(:clear)
      gitlab_keys.exec
    end

    it 'check-permissions arg should execute check_permissions method' do
      gitlab_keys = build_gitlab_keys('check-permissions')
      expect(gitlab_keys).to receive(:check_permissions)
      gitlab_keys.exec
    end

    it 'should puts message if unknown command arg' do
      gitlab_keys = build_gitlab_keys('change-key')
      expect(gitlab_keys).to receive(:puts).with('not allowed')
      gitlab_keys.exec
    end

    it 'should log a warning on unknown commands' do
      gitlab_keys = build_gitlab_keys('nooope')
      allow(gitlab_keys).to receive(:puts).and_return(nil)
      expect($logger).to receive(:warn).with("Attempt to execute invalid gitlab-keys command", command: '"nooope"')
      gitlab_keys.exec
    end
  end

  describe :lock do
    before do
      allow_any_instance_of(GitlabKeys).to receive(:lock_file).and_return(tmp_lock_file_path)
    end

    it "should raise exception if operation lasts more then timeout" do
      key = GitlabKeys.new
      expect do
        key.send :lock, 1 do
          sleep 2
        end
      end.to raise_error(Timeout::Error, 'execution expired')
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
      expect($global).to eq("foobar")
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
    allow(gitlab_keys).to receive(:auth_file).and_return(tmp_authorized_keys_path)
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
