require_relative 'spec_helper'

describe 'bin/gitlab-shell-authorized-keys-check' do
  def config_path
    File.join(ROOT_PATH, 'config.yml')
  end

  def tmp_config_path
    config_path + ".#{$$}"
  end

  def tmp_socket_path
    File.join(ROOT_PATH, 'tmp', 'gitlab-shell-authorized-keys-check-socket')
  end

  before(:all) do
    FileUtils.mkdir_p(File.dirname(tmp_socket_path))
    FileUtils.touch(File.join(ROOT_PATH, '.gitlab_shell_secret'))

    @server = HTTPUNIXServer.new(BindAddress: tmp_socket_path)
    @server.mount_proc('/api/v4/internal/authorized_keys') do |req, res|
      if req.query['key'] == 'known-rsa-key'
        res.status = 200
        res.content_type = 'application/json'
        res.body = '{"key":"known-rsa-key", "id": 1}'
      else
        res.status = 404
      end
    end

    @webrick_thread = Thread.new { @server.start }

    sleep(0.1) while @webrick_thread.alive? && @server.status != :Running
    raise "Couldn't start stub GitlabNet server" unless @server.status == :Running

    FileUtils.mv(config_path, tmp_config_path) if File.exist?(config_path)
    File.open(config_path, 'w') do |f|
      f.write("---\ngitlab_url: http+unix://#{CGI.escape(tmp_socket_path)}\n")
    end
  end

  after(:all) do
    @server.shutdown if @server
    @webrick_thread.join if @webrick_thread
    FileUtils.rm_f(config_path)
    FileUtils.mv(tmp_config_path, config_path) if File.exist?(tmp_config_path)
  end

  let(:gitlab_shell_path) { File.join(ROOT_PATH, 'bin', 'gitlab-shell') }
  let(:authorized_keys_check_path) { File.join(ROOT_PATH, 'bin', 'gitlab-shell-authorized-keys-check') }

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
