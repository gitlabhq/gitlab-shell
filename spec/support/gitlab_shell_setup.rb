require 'yaml'
require 'tempfile'

RSpec.shared_context 'gitlab shell', shared_context: :metadata do
  def original_root_path
    ROOT_PATH
  end

  def config_path
    File.join(tmp_root_path, 'config.yml')
  end

  def write_config(config)
    config['log_file'] ||= Tempfile.new.path

    File.open(config_path, 'w') do |f|
      f.write(config.to_yaml)
    end
  end

  def tmp_root_path
    @tmp_root_path ||= File.realpath(Dir.mktmpdir)
  end

  def mock_server(server)
    raise NotImplementedError.new(
      'mock_server method must be implemented in order to include gitlab shell context'
    )
  end

  # This has to be a relative path shorter than 100 bytes due to
  # limitations in how Unix sockets work.
  def tmp_socket_path
    'tmp/gitlab-shell-socket'
  end

  let(:gitlab_shell_path) { File.join(tmp_root_path, 'bin', 'gitlab-shell') }

  before(:all) do
    FileUtils.mkdir_p(File.dirname(tmp_socket_path))
    FileUtils.touch(File.join(tmp_root_path, '.gitlab_shell_secret'))

    @server = HTTPUNIXServer.new(BindAddress: tmp_socket_path)

    mock_server(@server)

    @webrick_thread = Thread.new { @server.start }

    sleep(0.1) while @webrick_thread.alive? && @server.status != :Running
    raise "Couldn't start stub GitlabNet server" unless @server.status == :Running
    system(original_root_path, 'bin/compile')

    FileUtils.rm_rf(File.join(tmp_root_path, 'bin'))
    FileUtils.cp_r('bin', tmp_root_path)
  end

  after(:all) do
    @server.shutdown if @server
    @webrick_thread.join if @webrick_thread
    FileUtils.rm_rf(tmp_root_path)
  end
end
