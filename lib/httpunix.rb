# support for http+unix://... connection scheme
#
# The URI scheme has the same structure as the similar one for python requests. See:
#   http://fixall.online/theres-no-need-to-reinvent-the-wheelhttpsgithubcommsabramorequests-unixsocketurl/241810/
#   https://github.com/msabramo/requests-unixsocket

require 'uri'
require 'net/http'

module URI
  class HTTPUNIX < HTTP
    def hostname
      # decode %XX from path to file
      v = host
      URI.decode(v) # rubocop:disable Lint/UriEscapeUnescape
    end

    # port is not allowed in URI
    DEFAULT_PORT = nil
    def set_port(value)
      return value unless value

      raise InvalidURIError, "http+unix:// cannot contain port"
    end
  end
  @@schemes['HTTP+UNIX'] = HTTPUNIX
end

# Based on:
# - http://stackoverflow.com/questions/15637226/ruby-1-9-3-simple-get-request-to-unicorn-through-socket
# - Net::HTTP::connect
module Net
  class HTTPUNIX < HTTP
    def initialize(socketpath, port = nil)
      super(socketpath, port)
      @port = nil # HTTP will set it to default - override back -> set DEFAULT_PORT
    end

    # override to prevent ":<port>" being appended to HTTP_HOST
    def addr_port
      address
    end

    def connect
      D "opening connection to #{address} ..."
      s = UNIXSocket.new(address)
      D "opened"
      @socket = BufferedIO.new(s)
      @socket.read_timeout = @read_timeout
      @socket.continue_timeout = @continue_timeout
      @socket.debug_output = @debug_output
      on_connect
    end
  end
end
