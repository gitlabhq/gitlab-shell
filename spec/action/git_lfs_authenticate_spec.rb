require_relative '../spec_helper'
require_relative '../../lib/action/git_lfs_authenticate'

describe Action::GitLFSAuthenticate do
  let(:key_id) { '1' }
  let(:repo_name) { 'gitlab-ci.git' }
  let(:key) { Actor::Key.new(key_id) }
  let(:username) { 'testuser' }
  let(:discover_payload) { { 'username' => username } }
  let(:api) { double(GitlabNet) }

  before do
    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(key_id).and_return(discover_payload)
  end

  subject do
    described_class.new(key, repo_name)
  end

  describe '#execute' do
    context 'when response from API is not a success' do
      before do
        expect(api).to receive(:lfs_authenticate).with(key_id, repo_name).and_return(nil)
      end

      it 'returns nil' do
        expect(subject.execute(nil, nil)).to be_nil
      end
    end

    context 'when response from API is a success' do
      let(:username) { 'testuser' }
      let(:lfs_token) { '1234' }
      let(:repository_http_path) { "/tmp/#{repo_name}" }
      let(:gitlab_lfs_authentication) { GitlabLfsAuthentication.new(username, lfs_token, repository_http_path) }

      before do
        expect(api).to receive(:lfs_authenticate).with(key_id, repo_name).and_return(gitlab_lfs_authentication)
      end

      it 'puts payload to stdout' do
        expect($stdout).to receive(:puts).with('{"header":{"Authorization":"Basic dGVzdHVzZXI6MTIzNA=="},"href":"/tmp/gitlab-ci.git/info/lfs/"}')
        expect(subject.execute(nil, nil)).to be_truthy
      end
    end
  end
end
