require 'spec_helper'
require 'gitlab_lfs_authentication'
require 'json'

describe GitlabLfsAuthentication do
  subject do
    GitlabLfsAuthentication.build_from_json(
      JSON.generate(
        {
          username: 'dzaporozhets',
          lfs_token: 'wsnys8Zm8Jn7zyhHTAAK',
          repository_http_path: 'http://gitlab.dev/repo'
        }
      )
    )
  end

  describe '#build_from_json' do
    it { expect(subject.username).to eq('dzaporozhets') }
    it { expect(subject.lfs_token).to eq('wsnys8Zm8Jn7zyhHTAAK') }
    it { expect(subject.repository_http_path).to eq('http://gitlab.dev/repo') }
  end

  describe '#authentication_payload' do
    result = "{\"header\":{\"Authorization\":\"Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL\"},\"href\":\"http://gitlab.dev/repo/info/lfs/\"}"

    it { expect(subject.authentication_payload).to eq(result) }

    it 'should be a proper JSON' do
      payload = subject.authentication_payload
      json_payload = JSON.parse(payload)

      expect(json_payload['header']['Authorization']).to eq('Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL')
      expect(json_payload['href']).to eq('http://gitlab.dev/repo/info/lfs/')
    end
  end
end
