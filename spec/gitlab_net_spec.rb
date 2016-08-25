require_relative 'spec_helper'
require_relative '../lib/gitlab_net'
require_relative '../lib/gitlab_access_status'


describe GitlabNet, vcr: true do
  let(:gitlab_net) { GitlabNet.new }
  let(:changes) { ['0000000000000000000000000000000000000000 92d0970eefd7acb6d548878925ce2208cfe2d2ec refs/heads/branch4'] }

  before do
    gitlab_net.stub(:host).and_return('https://dev.gitlab.org/api/v3/internal')
    gitlab_net.stub(:secret_token).and_return('a123')
  end

  describe :check do
    it 'should return 200 code for gitlab check' do
      VCR.use_cassette("check-ok") do
        result = gitlab_net.check
        result.code.should == '200'
      end
    end

    it 'adds the secret_token to request' do
      VCR.use_cassette("check-ok") do
        Net::HTTP::Get.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: 'a123'))
        gitlab_net.check
      end
    end

    it "raises an exception if the connection fails" do
      Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
      expect { gitlab_net.check }.to raise_error(GitlabNet::ApiUnreachableError)
    end
  end

  describe :discover do
    it 'should return user has based on key id' do
      VCR.use_cassette("discover-ok") do
        user = gitlab_net.discover('key-126')
        user['name'].should == 'Dmitriy Zaporozhets'
        user['lfs_token'].should == 'wsnys8Zm8Jn7zyhHTAAK'
        user['username'].should == 'dzaporozhets'
      end
    end

    it 'adds the secret_token to request' do
      VCR.use_cassette("discover-ok") do
        Net::HTTP::Get.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: 'a123'))
        gitlab_net.discover('key-126')
      end
    end

    it "raises an exception if the connection fails" do
      VCR.use_cassette("discover-ok") do
        Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
        expect { gitlab_net.discover('key-126') }.to raise_error(GitlabNet::ApiUnreachableError)
      end
    end
  end

  describe :broadcast_message do
    context "broadcast message exists" do
      it 'should return message' do
        VCR.use_cassette("broadcast_message-ok") do
          result = gitlab_net.broadcast_message
          result["message"].should == "Message"
        end
      end
    end

    context "broadcast message doesn't exist" do
      it 'should return nil' do
        VCR.use_cassette("broadcast_message-none") do
          result = gitlab_net.broadcast_message
          result.should == {}
        end
      end
    end
  end

  describe :authorized_key do
    let (:ssh_key) { "AAAAB3NzaC1yc2EAAAADAQABAAACAQDPKPqqnqQ9PDFw65cO7iHXrKw6ucSZg8Bd2CZ150Yy1YRDPJOWeRNCnddS+M/Lk" }

    it "should return nil when the resource is not implemented" do
      VCR.use_cassette("ssh-key-not-implemented") do
        result = gitlab_net.authorized_key("whatever")
        result.should be_nil
      end
    end

    it "should return nil when the fingerprint is not found" do
      VCR.use_cassette("ssh-key-not-found") do
        result = gitlab_net.authorized_key("whatever")
        result.should be_nil
      end
    end

    it "should return a ssh key with a valid fingerprint" do
      VCR.use_cassette("ssh-key-ok") do
        result = gitlab_net.authorized_key(ssh_key)
        result.should eq({
          "created_at" => "2016-03-04T18:27:36.959Z",
          "id" => 2,
          "key" => "ssh-rsa a-made=up-rsa-key dummy@gitlab.com",
          "title" => "some key title"
        })
      end
    end
  end

  describe '#two_factor_recovery_codes' do
    it 'returns two factor recovery codes' do
      VCR.use_cassette('two-factor-recovery-codes') do
        result = gitlab_net.two_factor_recovery_codes('key-1')
        expect(result['success']).to be_true
        expect(result['recovery_codes']).to eq(['f67c514de60c4953','41278385fc00c1e0'])
      end
    end

    it 'returns false when recovery codes cannot be generated' do
      VCR.use_cassette('two-factor-recovery-codes-fail') do
        result = gitlab_net.two_factor_recovery_codes('key-1')
        expect(result['success']).to be_false
        expect(result['message']).to eq('Could not find the given key')
      end
    end
  end

  describe :check_access do
    context 'ssh key with access to project' do
      it 'should allow pull access for dev.gitlab.org' do
        VCR.use_cassette("allowed-pull") do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-126', changes, 'ssh')
          access.allowed?.should be_true
          access.repository_http_path.should == 'http://gitlab.dev/gitlab/gitlabhq.git'
        end
      end

      it 'adds the secret_token to the request' do
        VCR.use_cassette("allowed-pull") do
          Net::HTTP::Post.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: 'a123'))
          gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-126', changes, 'ssh')
        end
      end

      it 'should allow push access for dev.gitlab.org' do
        VCR.use_cassette("allowed-push") do
          access = gitlab_net.check_access('git-upload-pack', 'gitlab/gitlabhq.git', 'key-126', changes, 'ssh')
          access.allowed?.should be_true
        end
      end
    end

    context 'ssh access has been disabled' do
      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette('ssh-access-disabled') do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'ssh')
          access.allowed?.should be_false
          access.message.should eq 'Git access over SSH is not allowed'
        end
      end

      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette('ssh-access-disabled') do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'ssh')
          access.allowed?.should be_false
          access.message.should eq 'Git access over SSH is not allowed'
        end
      end
    end

    context 'http access has been disabled' do
      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette('http-access-disabled') do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'http')
          access.allowed?.should be_false
          access.message.should eq 'Git access over HTTP is not allowed'
        end
      end

      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette('http-access-disabled') do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'http')
          access.allowed?.should be_false
          access.message.should eq 'Git access over HTTP is not allowed'
        end
      end
    end

    context 'ssh key without access to project' do
      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette("denied-pull") do
          access = gitlab_net.check_access('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'ssh')
          access.allowed?.should be_false
        end
      end

      it 'should deny push access for dev.gitlab.org' do
        VCR.use_cassette("denied-push") do
          access = gitlab_net.check_access('git-upload-pack', 'gitlab/gitlabhq.git', 'key-2', changes, 'ssh')
          access.allowed?.should be_false
        end
      end

      it 'should deny push access for dev.gitlab.org (with user)' do
        VCR.use_cassette("denied-push-with-user") do
          access = gitlab_net.check_access('git-upload-pack', 'gitlab/gitlabhq.git', 'user-1', changes, 'ssh')
          access.allowed?.should be_false
        end
      end
    end

    it "raises an exception if the connection fails" do
      Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
      expect {
        gitlab_net.check_access('git-upload-pack', 'gitlab/gitlabhq.git', 'user-1', changes, 'ssh')
      }.to raise_error(GitlabNet::ApiUnreachableError)
    end
  end

  describe :host do
    let(:net) { GitlabNet.new }
    subject { net.send :host }

    it { should include(net.send(:config).gitlab_url) }
    it("uses API version 3") { should include("api/v3") }
  end

  describe :http_client_for do
    subject { gitlab_net.send :http_client_for, URI('https://localhost/') }
    before do
      gitlab_net.stub :cert_store
      gitlab_net.send(:config).stub(:http_settings) { {'self_signed_cert' => true} }
    end

    its(:verify_mode) { should eq(OpenSSL::SSL::VERIFY_NONE) }
  end

  describe :http_request_for do
    let(:get) do
      double(Net::HTTP::Get).tap do |get|
        Net::HTTP::Get.stub(:new) { get }
      end
    end
    let(:user) { 'user' }
    let(:password) { 'password' }
    let(:url) { URI 'http://localhost/' }
    subject { gitlab_net.send :http_request_for, :get, url }

    before do
      gitlab_net.send(:config).http_settings.stub(:[]).with('user') { user }
      gitlab_net.send(:config).http_settings.stub(:[]).with('password') { password }
      get.should_receive(:basic_auth).with(user, password).once
      get.should_receive(:set_form_data).with(hash_including(secret_token: 'a123')).once
    end

    it { should_not be_nil }
  end

  describe :cert_store do
    let(:store) do
      double(OpenSSL::X509::Store).tap do |store|
        OpenSSL::X509::Store.stub(:new) { store }
      end
    end

    before :each do
      store.should_receive(:set_default_paths).once
    end

    after do
      gitlab_net.send :cert_store
    end

    it "calls add_file with http_settings['ca_file']" do
      gitlab_net.send(:config).http_settings.stub(:[]).with('ca_file') { 'test_file' }
      gitlab_net.send(:config).http_settings.stub(:[]).with('ca_path') { nil }
      store.should_receive(:add_file).with('test_file')
      store.should_not_receive(:add_path)
    end

    it "calls add_path with http_settings['ca_path']" do
      gitlab_net.send(:config).http_settings.stub(:[]).with('ca_file') { nil }
      gitlab_net.send(:config).http_settings.stub(:[]).with('ca_path') { 'test_path' }
      store.should_not_receive(:add_file)
      store.should_receive(:add_path).with('test_path')
    end
  end

  describe '#redis_client' do
    let(:config) { double('config') }

    context "with empty redis config" do
      it 'returns default parameters' do
        allow(gitlab_net).to receive(:config).and_return(config)
        allow(config).to receive(:redis).and_return( {} )

        expect_any_instance_of(Redis).to receive(:initialize).with({ host: '127.0.0.1',
                                                                     port: 6379,
                                                                     db: 0 })
        gitlab_net.redis_client
      end
    end

    context "with password" do
      it 'uses the specified host, port, and password' do
        allow(gitlab_net).to receive(:config).and_return(config)
        allow(config).to receive(:redis).and_return( { 'host' => 'localhost', 'port' => 1123, 'pass' => 'secret' } )

        expect_any_instance_of(Redis).to receive(:initialize).with({ host: 'localhost',
                                                                     port: 1123,
                                                                     db: 0,
                                                                     password: 'secret'})
        gitlab_net.redis_client
      end
    end

    context "with sentinels" do
      it 'uses the specified sentinels' do
        allow(gitlab_net).to receive(:config).and_return(config)
        allow(config).to receive(:redis).and_return({ 'host' => 'localhost', 'port' => 1123,
                                                      'sentinels' => [{'host' => '127.0.0.1', 'port' => 26380}] })

        expect_any_instance_of(Redis).to receive(:initialize).with({ host: 'localhost',
                                                                     port: 1123,
                                                                     db: 0,
                                                                     sentinels: [{host: '127.0.0.1', port: 26380}] })
        gitlab_net.redis_client
      end
    end


    context "with redis socket" do
      let(:socket) { '/tmp/redis.socket' }

      it 'uses the socket' do
        allow(gitlab_net).to receive(:config).and_return(config)
        allow(config).to receive(:redis).and_return( { 'socket' => socket })

        expect_any_instance_of(Redis).to receive(:initialize).with({ path: socket, db: 0 })
        gitlab_net.redis_client
      end
    end
  end
end
