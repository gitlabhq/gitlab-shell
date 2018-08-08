require_relative 'spec_helper'

describe 'bin/gitlab-shell-authorized-keys-check' do
  def original_root_path
    ROOT_PATH
  end

  # All this test boilerplate is mostly copy/pasted between
  # gitlab_shell_gitlab_shell_spec.rb and
  # gitlab_shell_authorized_keys_check_spec.rb
  def tmp_root_path
    @tmp_root_path ||= File.realpath(Dir.mktmpdir)
  end

  def config_path
    File.join(tmp_root_path, 'config.yml')
  end

  def tmp_socket_path
    # This has to be a relative path shorter than 100 bytes due to
    # limitations in how Unix sockets work.
    'tmp/gitlab-shell-authorized-keys-check-socket'
  end

  before(:all) do
    FileUtils.mkdir_p(File.dirname(tmp_socket_path))
    FileUtils.touch(File.join(tmp_root_path, '.gitlab_shell_secret'))

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

    File.open(config_path, 'w') do |f|
      f.write("---\ngitlab_url: http+unix://#{CGI.escape(tmp_socket_path)}\n")
    end

    copy_dirs = ['bin', 'lib']
    FileUtils.rm_rf(copy_dirs.map { |d| File.join(tmp_root_path, d) })
    FileUtils.cp_r(copy_dirs, tmp_root_path)
  end

  after(:all) do
    @server.shutdown if @server
    @webrick_thread.join if @webrick_thread
    FileUtils.rm_rf(tmp_root_path)
  end

  let(:gitlab_shell_path) { File.join(tmp_root_path, 'bin', 'gitlab-shell') }
  let(:authorized_keys_check_path) { File.join(tmp_root_path, 'bin', 'gitlab-shell-authorized-keys-check') }

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
