require_relative 'spec_helper'

describe 'bin/gitlab-shell-authorized-keys-check' do
  include_context 'gitlab shell'

  def tmp_socket_path
    # This has to be a relative path shorter than 100 bytes due to
    # limitations in how Unix sockets work.
    'tmp/gitlab-shell-authorized-keys-check-socket'
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/authorized_keys') do |req, res|
      if req.query['key'] == 'known-rsa-key'
        res.status = 200
        res.content_type = 'application/json'
        res.body = '{"key":"known-rsa-key", "id": 1}'
      else
        res.status = 404
      end
    end
  end

  let(:authorized_keys_check_path) { File.join(tmp_root_path, 'bin', 'gitlab-shell-authorized-keys-check') }

  shared_examples 'authorized keys check' do
    it 'succeeds when a valid key is given' do
      output, status = run!

      expect(output).to eq("command=\"#{gitlab_shell_path} key-1\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty known-rsa-key\n")
      expect(status).to be_success
    end

    it 'returns nothing when an unknown key is given' do
      output, status = run!(key: 'unknown-key')

      expect(output).to eq("# No key was found for unknown-key\n")
      expect(status).to be_success
    end

    it' fails when not enough arguments are given' do
      output, status = run!(key: nil)

      expect(output).to eq('')
      expect(status).not_to be_success
    end

    it' fails when too many arguments are given' do
      output, status = run!(key: ['a', 'b'])

      expect(output).to eq('')
      expect(status).not_to be_success
    end

    it 'skips when run as the wrong user' do
      output, status = run!(expected_user: 'unknown-user')

      expect(output).to eq('')
      expect(status).to be_success
    end

    it 'skips when the wrong users connects' do
      output, status = run!(actual_user: 'unknown-user')

      expect(output).to eq('')
      expect(status).to be_success
    end
  end

  describe 'without go features' do
    before(:all) do
      write_config(
        "gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}",
      )
    end

    it_behaves_like 'authorized keys check'
  end

  describe 'without go features (via go)', :go do
    before(:all) do
      write_config(
        "gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}",
      )
    end

    it_behaves_like 'authorized keys check'
  end

  describe 'with the go authorized-keys-check feature', :go do
    before(:all) do
      write_config(
        'gitlab_url' => "http+unix://#{CGI.escape(tmp_socket_path)}",
        'migration' => {
          'enabled' => true,
          'features' => ['authorized-keys-check']
        }
      )
    end

    it_behaves_like 'authorized keys check'
  end

  def run!(expected_user: 'git', actual_user: 'git', key: 'known-rsa-key')
    cmd = [
      authorized_keys_check_path,
      expected_user,
      actual_user,
      key
    ].flatten.compact

    output = IO.popen(cmd, &:read)

    [output, $?]
  end
end
