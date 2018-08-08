require_relative 'spec_helper'
require_relative '../lib/gitlab_config'

describe GitlabConfig do
  let(:config) { GitlabConfig.new }
  let(:config_data) do
    {
      # 'user' => 'git',
      # 'http_settings' => {
      #   'self_signed_cert' => false
      # },
      # 'log_level' => 'ERROR',
      # 'audit_usernames' => true,
      # 'log_format' => 'json', # Not sure on other values?
      # 'git_trace_log_file' => nil
    }
  end

  before do
    expect(YAML).to receive(:load_file).and_return(config_data)
  end

  describe '#gitlab_url' do
    let(:url) { 'http://test.com' }

    subject { config.gitlab_url }

    before { config_data['gitlab_url'] = url }

    it { should_not be_empty }
    it { should eq(url) }

    context 'remove trailing slashes' do
      before { config_data['gitlab_url'] = url + '//' }

      it { should eq(url) }
    end
  end

  describe '#audit_usernames' do
    subject { config.audit_usernames }

    it("returns false by default") { should eq(false) }
  end

  describe '#log_level' do
    subject { config.log_level }

    it 'returns "INFO" by default' do
      should eq('INFO')
    end
  end

  describe '#log_format' do
    subject { config.log_format }

    it 'returns "text" by default' do
      should eq('text')
    end
  end
end
