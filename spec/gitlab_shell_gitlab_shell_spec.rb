require_relative 'spec_helper'

require 'open3'

describe 'bin/gitlab-shell' do
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
    'tmp/gitlab-shell-socket'
  end

  before(:all) do
    FileUtils.mkdir_p(File.dirname(tmp_socket_path))
    FileUtils.touch(File.join(tmp_root_path, '.gitlab_shell_secret'))

    @server = HTTPUNIXServer.new(BindAddress: tmp_socket_path)
    @server.mount_proc('/api/v4/internal/discover') do |req, res|
      if req.query['key_id'] == '100' ||
         req.query['user_id'] == '10' ||
         req.query['username'] == 'someuser'
        res.status = 200
        res.content_type = 'application/json'
        res.body = '{"id":1, "name": "Some User", "username": "someuser"}'
      else
        res.status = 500
      end
    end

    @webrick_thread = Thread.new { @server.start }

    sleep(0.1) while @webrick_thread.alive? && @server.status != :Running
    raise "Couldn't start stub GitlabNet server" unless @server.status == :Running
    system(original_root_path, 'bin/compile')
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

  shared_examples 'results with keys' do
    # Basic valid input
    it 'succeeds and prints username when a valid known key id is given' do
      output, _, status = run!(["key-100"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints username when a valid known username is given' do
      output, _, status = run!(["username-someuser"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    # Valid but unknown input
    it 'succeeds and prints Anonymous when a valid unknown key id is given' do
      output, _, status = run!(["key-12345"])

      expect(output).to eq("Welcome to GitLab, Anonymous!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints Anonymous when a valid unknown username is given' do
      output, _, status = run!(["username-unknown"])

      expect(output).to eq("Welcome to GitLab, Anonymous!\n")
      expect(status).to be_success
    end

    it 'gets an ArgumentError on invalid input (empty)' do
      _, stderr, status = run!([])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    it 'gets an ArgumentError on invalid input (unknown)' do
      _, stderr, status = run!(["whatever"])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    it 'gets an ArgumentError on invalid input (multiple unknown)' do
      _, stderr, status = run!(["this", "is", "all", "invalid"])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    # Not so basic valid input
    # (https://gitlab.com/gitlab-org/gitlab-shell/issues/145)
    it 'succeeds and prints username when a valid known key id is given in the middle of other input' do
      output, _, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "key-100", "2foo"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints username when a valid known username is given in the middle of other input' do
      output, _, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-someuser" ,"foo"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end
  end

  describe 'without go features' do
    before(:context) do
      write_config("gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}")
    end

    it_behaves_like 'results with keys'
  end

  describe 'with the go discover feature', :go do
    before(:context) do
      write_config(
        "gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}",
        "migration" => { "enabled" => true,
                        "features" => ["discover"] }
      )
    end

    it_behaves_like 'results with keys' do
      before do
        pending
      end
    end

    it 'outputs "Only ssh allowed"' do
      _, stderr, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-someuser"], env: {})

      expect(stderr).to eq("Only ssh allowed\n")
      expect(status).not_to be_success
    end
  end

  def run!(args, env: {'SSH_CONNECTION' => 'fake'})
    cmd = [
      gitlab_shell_path,
      args
    ].flatten.compact.join(' ')

    Open3.capture3(env, cmd)
  end

  def write_config(config)
    File.open(config_path, 'w') do |f|
      f.write(config.to_yaml)
    end
  end
end
