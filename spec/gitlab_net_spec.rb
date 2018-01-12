require_relative 'spec_helper'
require_relative '../lib/gitlab_net'
require_relative '../lib/gitlab_access_status'

describe GitlabNet, vcr: true do
  let(:gitlab_net) { GitlabNet.new }
  let(:changes) { ['0000000000000000000000000000000000000000 92d0970eefd7acb6d548878925ce2208cfe2d2ec refs/heads/branch4'] }
  let(:host) { 'http://localhost:3000/api/v4/internal' }
  let(:project) { 'gitlab-org/gitlab-test.git' }
  let(:key) { 'key-1' }
  let(:key2) { 'key-2' }
  let(:secret) { "0a3938d9d95d807e94d937af3a4fbbea\n" }

  before do
    gitlab_net.stub(:host).and_return(host)
    gitlab_net.stub(:secret_token).and_return(secret)
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
        Net::HTTP::Get.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: secret))
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
        user = gitlab_net.discover(key)
        user['name'].should == 'Administrator'
        user['username'].should == 'root'
      end
    end

    it 'adds the secret_token to request' do
      VCR.use_cassette("discover-ok") do
        Net::HTTP::Get.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: secret))
        gitlab_net.discover(key)
      end
    end

    it "raises an exception if the connection fails" do
      VCR.use_cassette("discover-ok") do
        Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
        expect { gitlab_net.discover(key) }.to raise_error(GitlabNet::ApiUnreachableError)
      end
    end
  end

  describe '#lfs_authenticate' do
    context 'lfs authentication succeeded' do
      it 'should return the correct data' do
        VCR.use_cassette('lfs-authenticate-ok') do
          lfs_access = gitlab_net.lfs_authenticate(key, project)
          lfs_access.username.should == 'root'
          lfs_access.lfs_token.should == 'Hyzhyde_wLUeyUQsR3tHGTG8eNocVQm4ssioTEsBSdb6KwCSzQ'
          lfs_access.repository_http_path.should == URI.join(host.sub('api/v4', ''), project).to_s
        end
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

  describe :merge_request_urls do
    let(:gl_repository) { "project-1" }
    let(:changes) { "123456 789012 refs/heads/test\n654321 210987 refs/tags/tag" }
    let(:encoded_changes) { "123456%20789012%20refs/heads/test%0A654321%20210987%20refs/tags/tag" }

    it "sends the given arguments as encoded URL parameters" do
      gitlab_net.should_receive(:get).with("#{host}/merge_request_urls?project=#{project}&changes=#{encoded_changes}&gl_repository=#{gl_repository}")

      gitlab_net.merge_request_urls(gl_repository, project, changes)
    end

    it "omits the gl_repository parameter if it's nil" do
      gitlab_net.should_receive(:get).with("#{host}/merge_request_urls?project=#{project}&changes=#{encoded_changes}")

      gitlab_net.merge_request_urls(nil, project, changes)
    end

    it "returns an empty array when the result cannot be parsed as JSON" do
      response = double(:response, code: '200', body: '')
      allow(gitlab_net).to receive(:get).and_return(response)

      expect(gitlab_net.merge_request_urls(gl_repository, project, changes)).to eq([])
    end

    it "returns an empty array when the result's status is not 200" do
      response = double(:response, code: '500', body: '[{}]')
      allow(gitlab_net).to receive(:get).and_return(response)

      expect(gitlab_net.merge_request_urls(gl_repository, project, changes)).to eq([])
    end
  end

  describe :pre_receive do
    let(:gl_repository) { "project-1" }
    let(:params) { { gl_repository: gl_repository } }

    subject { gitlab_net.pre_receive(gl_repository) }

    it 'sends the correct parameters and returns the request body parsed' do
      Net::HTTP::Post.any_instance.should_receive(:set_form_data)
        .with(hash_including(params))

      VCR.use_cassette("pre-receive") { subject }
    end

    it 'calls /internal/pre-receive' do
      VCR.use_cassette("pre-receive") do
        expect(subject['reference_counter_increased']).to be(true)
      end
    end

    it 'throws a NotFound error when pre-receive is not available' do
      VCR.use_cassette("pre-receive-not-found") do
        expect { subject }.to raise_error(GitlabNet::NotFound)
      end
    end
  end

  describe :post_receive do
    let(:gl_repository) { "project-1" }
    let(:changes) { "123456 789012 refs/heads/test\n654321 210987 refs/tags/tag" }
    let(:params) do
      { gl_repository: gl_repository, identifier: key, changes: changes }
    end
    let(:merge_request_urls) do
      [{
        "branch_name" => "test",
        "url" => "http://localhost:3000/gitlab-org/gitlab-test/merge_requests/7",
        "new_merge_request" => false
      }]
    end

    subject { gitlab_net.post_receive(gl_repository, key, changes) }

    it 'sends the correct parameters' do
      Net::HTTP::Post.any_instance.should_receive(:set_form_data).with(hash_including(params))


      VCR.use_cassette("post-receive") do
        subject
      end
    end

    it 'calls /internal/post-receive' do
      VCR.use_cassette("post-receive") do
        expect(subject['merge_request_urls']).to eq(merge_request_urls)
        expect(subject['broadcast_message']).to eq('Message')
        expect(subject['reference_counter_decreased']).to eq(true)
      end
    end

    it 'throws a NotFound error when post-receive is not available' do
      VCR.use_cassette("post-receive-not-found") do
        expect { subject }.to raise_error(GitlabNet::NotFound)
      end
    end
  end

  describe :authorized_key do
    let (:ssh_key) { "rsa-key" }

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
          "can_push" => false,
          "created_at" => "2017-06-21T09:50:07.150Z",
          "id" => 99,
          "key" => "ssh-rsa rsa-key dummy@gitlab.com",
          "title" => "untitled"
        })
      end
    end
  end

  describe '#two_factor_recovery_codes' do
    it 'returns two factor recovery codes' do
      VCR.use_cassette('two-factor-recovery-codes') do
        result = gitlab_net.two_factor_recovery_codes(key)
        expect(result['success']).to be_true
        expect(result['recovery_codes']).to eq(['f67c514de60c4953','41278385fc00c1e0'])
      end
    end

    it 'returns false when recovery codes cannot be generated' do
      VCR.use_cassette('two-factor-recovery-codes-fail') do
        result = gitlab_net.two_factor_recovery_codes('key-777')
        expect(result['success']).to be_false
        expect(result['message']).to eq('Could not find the given key')
      end
    end
  end

  describe '#notify_post_receive' do
    let(:gl_repository) { 'project-1' }
    let(:repo_path) { '/path/to/my/repo.git' }
    let(:params) do
      { gl_repository: gl_repository, project: repo_path }
    end

    it 'sets the arguments as form parameters' do
      VCR.use_cassette('notify-post-receive') do
        Net::HTTP::Post.any_instance.should_receive(:set_form_data).with(hash_including(params))
        gitlab_net.notify_post_receive(gl_repository, repo_path)
      end
    end

    it 'returns true if notification was succesful' do
      VCR.use_cassette('notify-post-receive') do
        expect(gitlab_net.notify_post_receive(gl_repository, repo_path)).to be_true
      end
    end
  end

  describe :check_access do
    context 'ssh key with access nil, to project' do
      it 'should allow pull access for host' do
        VCR.use_cassette("allowed-pull") do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
          access.allowed?.should be_true
        end
      end

      it 'adds the secret_token to the request' do
        VCR.use_cassette("allowed-pull") do
          Net::HTTP::Post.any_instance.should_receive(:set_form_data).with(hash_including(secret_token: secret))
          gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
        end
      end

      it 'should allow push access for host' do
        VCR.use_cassette("allowed-push") do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'ssh')
          access.allowed?.should be_true
        end
      end
    end

    context 'ssh access has been disabled' do
      it 'should deny pull access for host' do
        VCR.use_cassette('ssh-pull-disabled') do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'ssh')
          access.allowed?.should be_false
          access.message.should eq 'Git access over SSH is not allowed'
        end
      end

      it 'should deny push access for host' do
        VCR.use_cassette('ssh-push-disabled') do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
          access.allowed?.should be_false
          access.message.should eq 'Git access over SSH is not allowed'
        end
      end
    end

    context 'http access has been disabled' do
      it 'should deny pull access for host' do
        VCR.use_cassette('http-pull-disabled') do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'http')
          access.allowed?.should be_false
          access.message.should eq 'Pulling over HTTP is not allowed.'
        end
      end

      it 'should deny push access for host' do
        VCR.use_cassette("http-push-disabled") do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'http')
          access.allowed?.should be_false
          access.message.should eq 'Pushing over HTTP is not allowed.'
        end
      end
    end

    context 'ssh key without access to project' do
      it 'should deny pull access for host' do
        VCR.use_cassette("ssh-pull-project-denied") do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key2, changes, 'ssh')
          access.allowed?.should be_false
        end
      end

      it 'should deny push access for host' do
        VCR.use_cassette("ssh-push-project-denied") do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key2, changes, 'ssh')
          access.allowed?.should be_false
        end
      end

      it 'should deny push access for host (with user)' do
        VCR.use_cassette("ssh-push-project-denied-with-user") do
          access = gitlab_net.check_access('git-upload-pack', nil, project, 'user-2', changes, 'ssh')
          access.allowed?.should be_false
        end
      end
    end

    it "raises an exception if the connection fails" do
      Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
      expect {
        gitlab_net.check_access('git-upload-pack', nil, project, 'user-1', changes, 'ssh')
      }.to raise_error(GitlabNet::ApiUnreachableError)
    end
  end

  describe :host do
    let(:net) { GitlabNet.new }
    subject { net.send :host }

    it { should include(net.send(:config).gitlab_url) }
    it("uses API version 4") { should include("api/v4") }
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
    context 'with stub' do
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
        get.should_receive(:set_form_data).with(hash_including(secret_token: secret)).once
      end

      it { should_not be_nil }
    end

    context 'Unix socket' do
      it 'sets the Host header to "localhost"' do
        gitlab_net = described_class.new
        gitlab_net.should_receive(:secret_token).and_return(secret)

        request = gitlab_net.send(:http_request_for, :get, URI('http+unix://%2Ffoo'))

        expect(request['Host']).to eq('localhost')
      end
    end
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
end
