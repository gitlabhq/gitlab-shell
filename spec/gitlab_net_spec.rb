require_relative 'spec_helper'
require_relative '../lib/gitlab_net'
require_relative '../lib/gitlab_access_status'

describe GitlabNet, vcr: true do
  using RSpec::Parameterized::TableSyntax

  let(:gitlab_net) { described_class.new }
  let(:changes) { ['0000000000000000000000000000000000000000 92d0970eefd7acb6d548878925ce2208cfe2d2ec refs/heads/branch4'] }
  let(:base_api_endpoint) { 'http://localhost:3000/api/v4' }
  let(:internal_api_endpoint) { 'http://localhost:3000/api/v4/internal' }
  let(:project) { 'gitlab-org/gitlab-test.git' }
  let(:key) { 'key-1' }
  let(:key2) { 'key-2' }
  let(:secret) { "0a3938d9d95d807e94d937af3a4fbbea\n" }

  before do
    $logger = double('logger').as_null_object
    allow(gitlab_net).to receive(:base_api_endpoint).and_return(base_api_endpoint)
    allow(gitlab_net).to receive(:secret_token).and_return(secret)
  end

  describe '#check' do
    it 'should return 200 code for gitlab check' do
      VCR.use_cassette("check-ok") do
        result = gitlab_net.check
        expect(result.code).to eq('200')
      end
    end

    it 'adds the secret_token to request' do
      VCR.use_cassette("check-ok") do
        expect_any_instance_of(Net::HTTP::Get).to receive(:set_form_data).with(hash_including(secret_token: secret))
        gitlab_net.check
      end
    end

    it "raises an exception if the connection fails" do
      allow_any_instance_of(Net::HTTP).to receive(:request).and_raise(StandardError)
      expect { gitlab_net.check }.to raise_error(GitlabNet::ApiUnreachableError)
    end
  end

  describe '#discover' do
    it 'should return user has based on key id' do
      VCR.use_cassette("discover-ok") do
        user = gitlab_net.discover(key)
        expect(user['name']).to eq('Administrator')
        expect(user['username']).to eq('root')
      end
    end

    it 'adds the secret_token to request' do
      VCR.use_cassette("discover-ok") do
        expect_any_instance_of(Net::HTTP::Get).to receive(:set_form_data).with(hash_including(secret_token: secret))
        gitlab_net.discover(key)
      end
    end

    it "raises an exception if the connection fails" do
      VCR.use_cassette("discover-ok") do
        allow_any_instance_of(Net::HTTP).to receive(:request).and_raise(StandardError)
        expect { gitlab_net.discover(key) }.to raise_error(GitlabNet::ApiUnreachableError)
      end
    end
  end

  describe '#lfs_authenticate' do
    context 'lfs authentication succeeded' do
      let(:repository_http_path) { URI.join(internal_api_endpoint.sub('/api/v4/internal', ''), project).to_s }

      context 'for download operation' do
        it 'should return the correct data' do
          VCR.use_cassette('lfs-authenticate-ok-download') do
            lfs_access = gitlab_net.lfs_authenticate(key, project, 'download')
            expect(lfs_access.username).to eq('root')
            expect(lfs_access.lfs_token).to eq('Hyzhyde_wLUeyUQsR3tHGTG8eNocVQm4ssioTEsBSdb6KwCSzQ')
            expect(lfs_access.repository_http_path).to eq(repository_http_path)
          end
        end
      end

      context 'for upload operation' do
        it 'should return the correct data' do
          VCR.use_cassette('lfs-authenticate-ok-upload') do
            lfs_access = gitlab_net.lfs_authenticate(key, project, 'upload')
            expect(lfs_access.username).to eq('root')
            expect(lfs_access.lfs_token).to eq('Hyzhyde_wLUeyUQsR3tHGTG8eNocVQm4ssioTEsBSdb6KwCSzQ')
            expect(lfs_access.repository_http_path).to eq(repository_http_path)
          end
        end
      end
    end
  end

  describe '#broadcast_message' do
    context "broadcast message exists" do
      it 'should return message' do
        VCR.use_cassette("broadcast_message-ok") do
          result = gitlab_net.broadcast_message
          expect(result["message"]).to eq("Message")
        end
      end
    end

    context "broadcast message doesn't exist" do
      it 'should return nil' do
        VCR.use_cassette("broadcast_message-none") do
          result = gitlab_net.broadcast_message
          expect(result).to eq({})
        end
      end
    end
  end

  describe '#merge_request_urls' do
    let(:gl_repository) { "project-1" }
    let(:changes) { "123456 789012 refs/heads/test\n654321 210987 refs/tags/tag" }
    let(:encoded_changes) { "123456%20789012%20refs/heads/test%0A654321%20210987%20refs/tags/tag" }

    it "sends the given arguments as encoded URL parameters" do
      expect(gitlab_net).to receive(:get).with("#{internal_api_endpoint}/merge_request_urls?project=#{project}&changes=#{encoded_changes}&gl_repository=#{gl_repository}")

      gitlab_net.merge_request_urls(gl_repository, project, changes)
    end

    it "omits the gl_repository parameter if it's nil" do
      expect(gitlab_net).to receive(:get).with("#{internal_api_endpoint}/merge_request_urls?project=#{project}&changes=#{encoded_changes}")

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

  describe '#pre_receive' do
    let(:gl_repository) { "project-1" }
    let(:params) { { gl_repository: gl_repository } }

    subject { gitlab_net.pre_receive(gl_repository) }

    it 'sends the correct parameters and returns the request body parsed' do
      expect_any_instance_of(Net::HTTP::Post).to receive(:set_form_data)
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

  describe '#post_receive' do
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
      expect_any_instance_of(Net::HTTP::Post).to receive(:set_form_data).with(hash_including(params))


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

  describe '#authorized_key' do
    let (:ssh_key) { "rsa-key" }

    it "should return nil when the resource is not implemented" do
      VCR.use_cassette("ssh-key-not-implemented") do
        result = gitlab_net.authorized_key("whatever")
        expect(result).to be_nil
      end
    end

    it "should return nil when the fingerprint is not found" do
      VCR.use_cassette("ssh-key-not-found") do
        result = gitlab_net.authorized_key("whatever")
        expect(result).to be_nil
      end
    end

    it "should return a ssh key with a valid fingerprint" do
      VCR.use_cassette("ssh-key-ok") do
        result = gitlab_net.authorized_key(ssh_key)
        expect(result).to eq({
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
        expect(result['success']).to be_truthy
        expect(result['recovery_codes']).to eq(['f67c514de60c4953','41278385fc00c1e0'])
      end
    end

    it 'returns false when recovery codes cannot be generated' do
      VCR.use_cassette('two-factor-recovery-codes-fail') do
        result = gitlab_net.two_factor_recovery_codes('key-777')
        expect(result['success']).to be_falsey
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
        expect_any_instance_of(Net::HTTP::Post).to receive(:set_form_data).with(hash_including(params))
        gitlab_net.notify_post_receive(gl_repository, repo_path)
      end
    end

    it 'returns true if notification was succesful' do
      VCR.use_cassette('notify-post-receive') do
        expect(gitlab_net.notify_post_receive(gl_repository, repo_path)).to be_truthy
      end
    end
  end

  describe '#check_access' do
    context 'ssh key with access nil, to project' do
      it 'should allow push access for host' do
        VCR.use_cassette("allowed-push") do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
          expect(access.allowed?).to be_truthy
        end
      end

      context 'but project not found' do
        where(:desc, :cassette, :message) do
          'deny push access for host'                                        | 'allowed-push-project-not-found'                | 'The project you were looking for could not be found.'
          'deny push access for host (when text/html)'                       | 'allowed-push-project-not-found-text-html'      | 'API is not accessible'
          'deny push access for host (when text/plain)'                      | 'allowed-push-project-not-found-text-plain'     | 'API is not accessible'
          'deny push access for host (when 404 is returned)'                 | 'allowed-push-project-not-found-404'            | 'The project you were looking for could not be found.'
          'deny push access for host (when 404 is returned with text/html)'  | 'allowed-push-project-not-found-404-text-html'  | 'API is not accessible'
          'deny push access for host (when 404 is returned with text/plain)' | 'allowed-push-project-not-found-404-text-plain' | 'API is not accessible'
        end

        with_them do
          it 'should deny push access for host' do
            VCR.use_cassette(cassette) do
              access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
              expect(access.allowed?).to be_falsey
              expect(access.message).to eql(message)
            end
          end
        end
      end

      it 'adds the secret_token to the request' do
        VCR.use_cassette("allowed-push") do
          expect_any_instance_of(Net::HTTP::Post).to receive(:set_form_data).with(hash_including(secret_token: secret))
          gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
        end
      end

      it 'should allow pull access for host' do
        VCR.use_cassette("allowed-pull") do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'ssh')
          expect(access.allowed?).to be_truthy
        end
      end
    end

    context 'ssh access has been disabled' do
      it 'should deny pull access for host' do
        VCR.use_cassette('ssh-pull-disabled') do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'ssh')
          expect(access.allowed?).to be_falsey
          expect(access.message).to eq 'Git access over SSH is not allowed'
        end
      end

      it 'should deny push access for host' do
        VCR.use_cassette('ssh-push-disabled') do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'ssh')
          expect(access.allowed?).to be_falsey
          expect(access.message).to eq 'Git access over SSH is not allowed'
        end
      end
    end

    context 'http access has been disabled' do
      it 'should deny pull access for host' do
        VCR.use_cassette('http-pull-disabled') do
          access = gitlab_net.check_access('git-upload-pack', nil, project, key, changes, 'http')
          expect(access.allowed?).to be_falsey
          expect(access.message).to eq 'Pulling over HTTP is not allowed.'
        end
      end

      it 'should deny push access for host' do
        VCR.use_cassette("http-push-disabled") do
          access = gitlab_net.check_access('git-receive-pack', nil, project, key, changes, 'http')
          expect(access.allowed?).to be_falsey
          expect(access.message).to eq 'Pushing over HTTP is not allowed.'
        end
      end
    end

    context 'ssh key without access to project' do
      where(:desc, :cassette, :message) do
        'deny push access for host'                                        | 'ssh-push-project-denied'                | 'Git access over SSH is not allowed'
        'deny push access for host (when 401 is returned)'                 | 'ssh-push-project-denied-401'            | 'Git access over SSH is not allowed'
        'deny push access for host (when 401 is returned with text/html)'  | 'ssh-push-project-denied-401-text-html'  | 'API is not accessible'
        'deny push access for host (when 401 is returned with text/plain)' | 'ssh-push-project-denied-401-text-plain' | 'API is not accessible'
        'deny pull access for host'                                        | 'ssh-pull-project-denied'                | 'Git access over SSH is not allowed'
        'deny pull access for host (when 401 is returned)'                 | 'ssh-pull-project-denied-401'            | 'Git access over SSH is not allowed'
        'deny pull access for host (when 401 is returned with text/html)'  | 'ssh-pull-project-denied-401-text-html'  | 'API is not accessible'
        'deny pull access for host (when 401 is returned with text/plain)' | 'ssh-pull-project-denied-401-text-plain' | 'API is not accessible'
      end

      with_them do
        it 'should deny push access for host' do
          VCR.use_cassette(cassette) do
            access = gitlab_net.check_access('git-receive-pack', nil, project, key2, changes, 'ssh')
            expect(access.allowed?).to be_falsey
            expect(access.message).to eql(message)
          end
        end
      end

      it 'should deny pull access for host (with user)' do
        VCR.use_cassette("ssh-pull-project-denied-with-user") do
          access = gitlab_net.check_access('git-upload-pack', nil, project, 'user-2', changes, 'ssh')
          expect(access.allowed?).to be_falsey
          expect(access.message).to eql('Git access over SSH is not allowed')
        end
      end
    end

    it 'handles non 200 status codes' do
      resp = double(:resp, code: 501)

      allow(gitlab_net).to receive(:post).and_return(resp)

      access = gitlab_net.check_access('git-upload-pack', nil, project, 'user-2', changes, 'ssh')
      expect(access).not_to be_allowed
    end

    it "raises an exception if the connection fails" do
      allow_any_instance_of(Net::HTTP).to receive(:request).and_raise(StandardError)
      expect {
        gitlab_net.check_access('git-upload-pack', nil, project, 'user-1', changes, 'ssh')
      }.to raise_error(GitlabNet::ApiUnreachableError)
    end
  end

  describe '#base_api_endpoint' do
    let(:net) { described_class.new }

    subject { net.send :base_api_endpoint }

    it { is_expected.to include(net.send(:config).gitlab_url) }
    it("uses API version 4") { is_expected.to end_with("api/v4") }
  end

  describe '#internal_api_endpoint' do
    let(:net) { described_class.new }

    subject { net.send :internal_api_endpoint }

    it { is_expected.to include(net.send(:config).gitlab_url) }
    it("uses API version 4") { is_expected.to end_with("api/v4/internal") }
  end

  describe '#http_client_for' do
    subject { gitlab_net.send :http_client_for, URI('https://localhost/') }

    before do
      allow(gitlab_net).to receive :cert_store
      allow(gitlab_net.send(:config)).to receive(:http_settings) { {'self_signed_cert' => true} }
    end

    it { expect(subject.verify_mode).to eq(OpenSSL::SSL::VERIFY_NONE) }
  end

  describe '#http_request_for' do
    context 'with stub' do
      let(:get) { double(Net::HTTP::Get) }
      let(:user) { 'user' }
      let(:password) { 'password' }
      let(:url) { URI 'http://localhost/' }
      let(:params) { { 'key1' => 'value1' } }
      let(:headers) { { 'Content-Type' => 'application/json'} }
      let(:options) { { json: { 'key2' => 'value2' } } }

      context 'with no params, options or headers' do
        subject { gitlab_net.send :http_request_for, :get, url }

        before do
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('user') { user }
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('password') { password }
          expect(Net::HTTP::Get).to receive(:new).with('/', {}).and_return(get)
          expect(get).to receive(:basic_auth).with(user, password).once
          expect(get).to receive(:set_form_data).with(hash_including(secret_token: secret)).once
        end

        it { is_expected.not_to be_nil }
      end

      context 'with params' do
        subject { gitlab_net.send :http_request_for, :get, url, params: params, headers: headers }

        before do
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('user') { user }
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('password') { password }
          expect(Net::HTTP::Get).to receive(:new).with('/', headers).and_return(get)
          expect(get).to receive(:basic_auth).with(user, password).once
          expect(get).to receive(:set_form_data).with({ 'key1' => 'value1', secret_token: secret }).once
        end

        it { is_expected.not_to be_nil }
      end

      context 'with headers' do
        subject { gitlab_net.send :http_request_for, :get, url, headers: headers }

        before do
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('user') { user }
          allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('password') { password }
          expect(Net::HTTP::Get).to receive(:new).with('/', headers).and_return(get)
          expect(get).to receive(:basic_auth).with(user, password).once
          expect(get).to receive(:set_form_data).with(hash_including(secret_token: secret)).once
        end

        it { is_expected.not_to be_nil }
      end

      context 'with options' do
        context 'with json' do
          subject { gitlab_net.send :http_request_for, :get, url, options: options }

          before do
            allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('user') { user }
            allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('password') { password }
            expect(Net::HTTP::Get).to receive(:new).with('/', {}).and_return(get)
            expect(get).to receive(:basic_auth).with(user, password).once
            expect(get).to receive(:body=).with({ 'key2' => 'value2', secret_token: secret }.to_json).once
            expect(get).not_to receive(:set_form_data)
          end

          it { is_expected.not_to be_nil }
        end
      end
    end

    context 'Unix socket' do
      it 'sets the Host header to "localhost"' do
        gitlab_net = described_class.new
        expect(gitlab_net).to receive(:secret_token).and_return(secret)

        request = gitlab_net.send(:http_request_for, :get, URI('http+unix://%2Ffoo'))

        expect(request['Host']).to eq('localhost')
      end
    end
  end

  describe '#cert_store' do
    let(:store) do
      double(OpenSSL::X509::Store).tap do |store|
        allow(OpenSSL::X509::Store).to receive(:new) { store }
      end
    end

    before :each do
      expect(store).to receive(:set_default_paths).once
    end

    after do
      gitlab_net.send :cert_store
    end

    it "calls add_file with http_settings['ca_file']" do
      allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('ca_file') { 'test_file' }
      allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('ca_path') { nil }
      expect(store).to receive(:add_file).with('test_file')
      expect(store).not_to receive(:add_path)
    end

    it "calls add_path with http_settings['ca_path']" do
      allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('ca_file') { nil }
      allow(gitlab_net.send(:config).http_settings).to receive(:[]).with('ca_path') { 'test_path' }
      expect(store).not_to receive(:add_file)
      expect(store).to receive(:add_path).with('test_path')
    end
  end
end
