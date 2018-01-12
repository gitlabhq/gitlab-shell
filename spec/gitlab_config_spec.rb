require_relative 'spec_helper'
require_relative '../lib/gitlab_config'

describe GitlabConfig do
  let(:config) { GitlabConfig.new }

  describe :gitlab_url do
    let(:url) { 'http://test.com' }
    subject { config.gitlab_url }
    before { config.send(:config)['gitlab_url'] = url }

    it { should_not be_empty }
    it { should eq(url) }

    context 'remove trailing slashes' do
      before { config.send(:config)['gitlab_url'] = url + '//' }

      it { should eq(url) }
    end
  end

  describe :audit_usernames do
    subject { config.audit_usernames }

    it("returns false by default") { should eq(false) }
  end
end
