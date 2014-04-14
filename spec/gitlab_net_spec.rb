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
    end
  end
end
