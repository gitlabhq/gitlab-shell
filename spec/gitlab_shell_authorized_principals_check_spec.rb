require_relative 'spec_helper'

describe 'bin/gitlab-shell-authorized-principals-check' do
  include_context 'gitlab shell'

  before(:all) do
    write_config({})
  end

  def mock_server(server)
    # Do nothing as we're not connecting to a server in this check.
  end

  let(:authorized_principals_check_path) { File.join(tmp_root_path, 'bin', 'gitlab-shell-authorized-principals-check') }

  describe 'authorized principals check' do
    it 'succeeds when a valid principal is given' do
      output, status = run!

      expect(output).to eq("command=\"#{gitlab_shell_path} username-key\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty principal\n")
      expect(status).to be_success
    end

    it 'fails when not enough arguments are given' do
      output, status = run!(key_id: nil, principals: [])

      expect(output).to eq('')
      expect(status).not_to be_success
    end

    it 'fails when key_id is blank' do
      output, status = run!(key_id: '')

      expect(output).to eq('')
      expect(status).not_to be_success
    end

    it 'fails when principals include an empty item' do
      output, status = run!(principals: ['principal', ''])

      expect(output).to eq('')
      expect(status).not_to be_success
    end
  end

  def run!(key_id: 'key', principals: ['principal'])
    cmd = [
      authorized_principals_check_path,
      key_id,
      principals,
    ].flatten.compact

    output = IO.popen(cmd, &:read)

    [output, $?]
  end
end
