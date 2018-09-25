require 'spec_helper'
require 'gitlab_access'

describe GitlabAccess do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name) { 'dzaporozhets/gitlab-ci' }
  let(:repo_path) { File.join(repository_path, repo_name) + ".git" }
  let(:api) do
    double(GitlabNet).tap do |api|
      allow(api).to receive(:check_access).and_return(GitAccessStatus.new(true,
                                                '200',
                                                 'ok',
                                                 gl_repository: 'project-1',
                                                 gl_id: 'user-123',
                                                 gl_username: 'testuser',
                                                 git_config_options: ['receive.MaxInputSize=10000'],
                                                 gitaly: nil,
                                                 git_protocol: 'version=2'))
    end
  end
  subject do
    GitlabAccess.new(nil, repo_path, 'key-123', 'wow', 'ssh').tap do |access|
      allow(access).to receive(:exec_cmd).and_return(:exec_called)
      allow(access).to receive(:api).and_return(api)
    end
  end

  before do
    allow_any_instance_of(GitlabConfig).to receive(:repos_path).and_return(repository_path)
  end

  describe :initialize do
    it { expect(subject.repo_path).to eq(repo_path) }
    it { expect(subject.changes).to eq(['wow']) }
    it { expect(subject.protocol).to eq('ssh') }
  end

  describe "#exec" do
    context "access is granted" do
      it "returns true" do
        expect(subject.exec).to be_truthy
      end
    end

    context "access is denied" do
      before do
        allow(api).to receive(:check_access).and_return(GitAccessStatus.new(
                  false,
                  '401',
                  'denied',
                  gl_repository: nil,
                  gl_id: nil,
                  gl_username: nil,
                  git_config_options: nil,
                  gitaly: nil,
                  git_protocol: nil
                ))
      end

      it "returns false" do
        expect(subject.exec).to be_falsey
      end
    end

    context "API connection fails" do
      before do
        allow(api).to receive(:check_access).and_raise(GitlabNet::ApiUnreachableError)
      end

      it "returns false" do
        expect(subject.exec).to be_falsey
      end
    end
  end
end
