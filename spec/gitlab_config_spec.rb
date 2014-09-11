require_relative 'spec_helper'
require_relative '../lib/gitlab_config'

describe GitlabConfig do
  let(:config) { GitlabConfig.new }

  describe :redis do
    subject { config.redis }

    it { should be_a(Hash) }
    it { should have_key('bin') }
    it { should have_key('host') }
    it { should have_key('port') }
    it { should have_key('namespace') }
  end

  describe :redis_namespace do
    subject { config.redis_namespace }

    it { should eq('resque:gitlab') }
  end

  describe :gitlab_url do
    let(:url) { 'http://test.com' }
    subject { config.gitlab_url }
    before { config.send(:config)['gitlab_url'] = url }

    it { should_not be_empty }
    it { should eq(url) }
  end

  describe :audit_usernames do
    subject { config.audit_usernames }

    it("returns false by default") { should eq(false) }
  end

  describe :redis_command do
    subject { config.redis_command }

    it { should be_an(Array) }
    it { should include(config.redis['host']) }
    it { should include(config.redis['bin']) }
    it { should include(config.redis['port'].to_s) }

    context "with empty redis config" do
      before do
        config.stub(:redis) { {} }
      end

      it { should be_an(Array) }
      it { should include('redis-cli') }
    end

    context "with redis socket" do
      let(:socket) { '/tmp/redis.socket' }
      before do
        config.stub(:redis) { {'bin' => '', 'socket' => socket } }
      end

      it { should be_an(Array) }
      it { should include(socket) }
      it { should_not include('-p') }
      it { should_not include('-h') }
    end
  end
end
