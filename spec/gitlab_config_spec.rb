require_relative 'spec_helper'
require_relative '../lib/gitlab_config'

describe GitlabConfig do
  let(:config) { GitlabConfig.new }

  describe :redis do
    before do
      config_file = File.read('spec/fixtures/gitlab_config_redis.yml')
      config.instance_variable_set(:@config, YAML.load(config_file))
    end

    it { config.redis['bin'].should eq('/usr/bin/redis-cli') }
    it { config.redis['host'].should eq('127.0.1.1') }
    it { config.redis['port'].should eq(6378) }
    it { config.redis['database'].should eq(1) }
    it { config.redis['namespace'].should eq('my:gitlab') }
    it { config.redis['socket'].should eq('/var/run/redis/redis.sock') }
    it { config.redis['pass'].should eq('secure') }
    it { config.redis['sentinels'].should eq([{ 'host' => '127.0.0.1', 'port' => 26380 }]) }
  end

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
