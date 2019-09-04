require_relative 'spec_helper'
require_relative '../lib/gitlab_keys'
require 'stringio'

describe GitlabKeys do
  before do
    $logger = double('logger').as_null_object
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
end
