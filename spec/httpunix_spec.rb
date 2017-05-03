require_relative 'spec_helper'
require_relative '../lib/httpunix'
require 'webrick'

describe URI::HTTPUNIX do
  describe :parse do
    uri = URI::parse('http+unix://%2Fpath%2Fto%2Fsocket/img.jpg')
    subject { uri }

    it { should be_an_instance_of(URI::HTTPUNIX) }
    its(:scheme) { should eq('http+unix') }
    its(:hostname) { should eq('/path/to/socket') }
    its(:path) { should eq('/img.jpg') }
  end
end


# like WEBrick::HTTPServer, but listens on UNIX socket
class HTTPUNIXServer < WEBrick::HTTPServer
  def listen(address, port)
    socket = Socket.unix_server_socket(address)
    socket.autoclose = false
    server = UNIXServer.for_fd(socket.fileno)
    socket.close
    @listeners << server
  end

  # Workaround:
  # https://bugs.ruby-lang.org/issues/10956
  # Affecting Ruby 2.2
  # Fix for 2.2 is at https://github.com/ruby/ruby/commit/ab0a64e1
  # However, this doesn't work with 2.1. The following should work for both:
  def start(&block)
    @shutdown_pipe = IO.pipe
    shutdown_pipe = @shutdown_pipe
    super(&block)
  end

  def cleanup_shutdown_pipe(shutdown_pipe)
    @shutdown_pipe = nil
    return if !shutdown_pipe
    super(shutdown_pipe)
  end
end

def tmp_socket_path
  'tmp/test-socket'
end

describe Net::HTTPUNIX do
  # "hello world" over unix socket server in background thread
  FileUtils.mkdir_p(File.dirname(tmp_socket_path))
  server = HTTPUNIXServer.new(:BindAddress => tmp_socket_path)
  server.mount_proc '/' do |req, resp|
    resp.body = "Hello World  (at #{req.path})"
  end
  Thread.start { server.start }

  it "talks via HTTP ok" do
    VCR.turned_off do
      begin
        WebMock.allow_net_connect!
        http = Net::HTTPUNIX.new(tmp_socket_path)
        expect(http.get('/').body).to eq('Hello World  (at /)')
        expect(http.get('/path').body).to eq('Hello World  (at /path)')

      ensure
        WebMock.disable_net_connect!
      end
    end
  end
end
