require_relative 'spec_helper'
require_relative '../lib/gitlab_net'


describe GitlabNet do
  describe :allowed? do
    let(:gitlab_net) { GitlabNet.new }

    before do
      gitlab_net.stub!(:host).and_return('https://dev.gitlab.org/api/v3/internal')
    end

    it 'should allow pull access for dev.gitlab.org', vcr: true do
      VCR.use_cassette("allowed-pull") do
        access = gitlab_net.allowed?('git-receive-pack', 'gitlab/gitlabhq.git', 'key-1', 'master')
        access.should be_true
      end
    end

    it 'should allow push access for dev.gitlab.org', vcr: true do
      VCR.use_cassette("allowed-push") do
        access = gitlab_net.allowed?('git-upload-pack', 'gitlab/gitlabhq.git', 'key-1', 'master')
        access.should be_true
      end
    end
  end
end
