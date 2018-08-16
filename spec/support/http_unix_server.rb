require 'webrick'

# like WEBrick::HTTPServer, but listens on UNIX socket
class HTTPUNIXServer < WEBrick::HTTPServer
  def initialize(config = {})
    null_log = WEBrick::Log.new(IO::NULL, 7)

    super(config.merge(Logger: null_log, AccessLog: null_log))
  end

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
