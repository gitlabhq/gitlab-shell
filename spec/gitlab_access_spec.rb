require 'spec_helper'
require 'gitlab_access'

describe GitlabAccess do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name) { 'dzaporozhets/gitlab-ci' }
  let(:repo_path) { File.join(repository_path, repo_name) + ".git" }
  let(:api) do
    double(GitlabNet).tap do |api|
      allow(api).to receive(:check_access).and_return(
        Action::Gitaly.new(
          'key-1',
          'project-1',
          'testuser',
          ['receive.MaxInputSize=10000'],
          'version=2',
          '/home/git/repositories',
          nil
        )
      )
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

  describe "#exec" do
    context "access is granted" do
      it "returns true" do
        expect(subject.exec).to be_truthy
      end
    end

    context "access is denied" do
      before do
        allow(api).to receive(:check_access).and_raise(AccessDeniedError)
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
