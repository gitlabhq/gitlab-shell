require_relative 'spec_helper'
require_relative '../lib/httpunix'

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

describe Net::HTTPUNIX do
  def tmp_socket_path
    File.join(ROOT_PATH, 'tmp/test-socket')
  end

  before(:all) do
    # "hello world" over unix socket server in background thread
    FileUtils.mkdir_p(File.dirname(tmp_socket_path))
    @server = HTTPUNIXServer.new(BindAddress: tmp_socket_path)
    @server.mount_proc '/' do |req, resp|
      resp.body = "Hello World  (at #{req.path})"
    end

    @webrick_thread = Thread.new { @server.start }

    sleep(0.1) while @webrick_thread.alive? && @server.status != :Running
    raise "Couldn't start HTTPUNIXServer" unless @server.status == :Running
  end

  after(:all) do
    @server.shutdown if @server
    @webrick_thread.join if @webrick_thread
  end

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
