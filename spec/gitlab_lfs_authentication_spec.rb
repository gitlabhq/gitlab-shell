require 'spec_helper'
require 'gitlab_lfs_authentication'
require 'json'

describe GitlabLfsAuthentication do
  let(:payload_from_gitlab_api) do
    {
      username: 'dzaporozhets',
      lfs_token: 'wsnys8Zm8Jn7zyhHTAAK',
      repository_http_path: 'http://gitlab.dev/repo'
    }
  end

  subject do
    GitlabLfsAuthentication.build_from_json(
      JSON.generate(payload_from_gitlab_api)
    )
  end

  describe '#build_from_json' do
    it { expect(subject.username).to eq('dzaporozhets') }
    it { expect(subject.lfs_token).to eq('wsnys8Zm8Jn7zyhHTAAK') }
    it { expect(subject.repository_http_path).to eq('http://gitlab.dev/repo') }
  end

  describe '#authentication_payload' do
    shared_examples 'a valid payload' do
      it 'should be proper JSON' do
        payload = subject.authentication_payload
        json_payload = JSON.parse(payload)

        expect(json_payload['header']['Authorization']).to eq('Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL')
        expect(json_payload['href']).to eq('http://gitlab.dev/repo/info/lfs')
      end
    end

    context 'without expires_in' do
      let(:result) { { 'header' => { 'Authorization' => 'Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL' }, 'href' => 'http://gitlab.dev/repo/info/lfs' }.to_json }

      it { expect(subject.authentication_payload).to eq(result) }

      it_behaves_like 'a valid payload'
    end

    context 'with expires_in' do
      let(:result) { { 'header' => { 'Authorization' => 'Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL' }, 'href' => 'http://gitlab.dev/repo/info/lfs', 'expires_in' => 1800 }.to_json }

      before do
        payload_from_gitlab_api[:expires_in] = 1800
      end

      it { expect(subject.authentication_payload).to eq(result) }

      it_behaves_like 'a valid payload'
    end
  end
end
