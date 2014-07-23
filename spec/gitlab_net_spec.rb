require_relative 'spec_helper'
require_relative '../lib/gitlab_net'


describe GitlabNet, vcr: true do
  let(:gitlab_net) { GitlabNet.new }

  before do
    gitlab_net.stub!(:host).and_return('https://dev.gitlab.org/api/v3/internal')
  end

  describe :check do
    it 'should return 200 code for gitlab check' do
      VCR.use_cassette("check-ok") do
        result = gitlab_net.check
        result.code.should == '200'
      end
    end
  end

  describe :discover do
    it 'should return user has based on key id' do
      VCR.use_cassette("discover-ok") do
        user = gitlab_net.discover('key-126')
        user['name'].should == 'Dmitriy Zaporozhets'
      end
    end
  end

  describe :allowed? do
    context 'ssh key with access to project' do
      it 'should allow pull access for dev.gitlab.org' do
        VCR.use_cassette("allowed-pull") do
          access = gitlab_net.allowed?('git-receive-pack', 'gitlab/gitlabhq.git', 'key-126', 'master')
          access.should be_true
        end
      end

      it 'should allow push access for dev.gitlab.org' do
        VCR.use_cassette("allowed-push") do
          access = gitlab_net.allowed?('git-upload-pack', 'gitlab/gitlabhq.git', 'key-126', 'master')
          access.should be_true
        end
      end
    end

    context 'ssh key without access to project' do
      it 'should deny pull access for dev.gitlab.org' do
        VCR.use_cassette("denied-pull") do
          access = gitlab_net.allowed?('git-receive-pack', 'gitlab/gitlabhq.git', 'key-2', 'master')
          access.should be_false
        end
      end

      it 'should deny push access for dev.gitlab.org' do
        VCR.use_cassette("denied-push") do
          access = gitlab_net.allowed?('git-upload-pack', 'gitlab/gitlabhq.git', 'key-2', 'master')
          access.should be_false
        end
      end

      it 'should deny push access for dev.gitlab.org (with user)' do
        VCR.use_cassette("denied-push-with-user") do
          access = gitlab_net.allowed?('git-upload-pack', 'gitlab/gitlabhq.git', 'user-1', 'master')
          access.should be_false
        end
      end
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
      gitlab_net.send(:config).http_settings.stub(:[]).with('self_signed_cert') { true }
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
    subject { gitlab_net.send :http_request_for, url }

    before do
      gitlab_net.send(:config).http_settings.stub(:[]).with('user') { user }
      gitlab_net.send(:config).http_settings.stub(:[]).with('password') { password }
      get.should_receive(:basic_auth).with(user, password).once
    end

    it { should_not be_nil }
  end
end

