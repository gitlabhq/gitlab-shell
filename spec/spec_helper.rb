ROOT_PATH = File.expand_path(File.join(File.dirname(__FILE__), ".."))

require 'simplecov'
SimpleCov.start

require 'vcr'
require 'webmock'
require 'webrick'

VCR.configure do |c|
  c.cassette_library_dir = 'spec/vcr_cassettes'
  c.hook_into :webmock
  c.configure_rspec_metadata!
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
