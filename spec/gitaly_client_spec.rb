require_relative 'spec_helper'
require_relative '../lib/gitaly_client'


describe GitalyClient, vcr: true do
  include WebMock::API

  let(:gitaly_client) { GitalyClient.new('http+unix://%2Fpath%2Fto%2Fgitaly.socket') }
  let(:repo_url) { 'gitlab/gitlabhq.git' }

  describe :notify_post_receive do
    before do
      stub_request(:post, /.*/)
    end

    it 'should return 200 code for gitlab check' do
      Net::HTTP::Post.any_instance.should_receive(:set_form_data).with(project: repo_url)
      gitaly_client.notify_post_receive(repo_url)
    end

    it "raises an exception if the connection fails" do
      Net::HTTP.any_instance.stub(:request).and_raise(StandardError)
      expect { gitaly_client.notify_post_receive(repo_url) }.to raise_error(GitalyClient::ApiUnreachableError)
    end

    after do
      WebMock.reset!
    end
  end
end
