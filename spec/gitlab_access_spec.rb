require 'spec_helper'
require 'gitlab_access'

describe GitlabAccess do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name) { 'dzaporozhets/gitlab-ci' }
  let(:repo_path) { File.join(repository_path, repo_name) + ".git" }
  let(:api) do
    double(GitlabNet).tap do |api|
      api.stub(check_access: GitAccessStatus.new(true,
                                                 'ok',
                                                 gl_repository: 'project-1',
                                                 gl_username: 'testuser',
                                                 repository_path: '/home/git/repositories',
                                                 gitaly: nil))
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
    it { expect(subject.send(:repo_path)).to eql repo_path } # FIXME: don't access private instance variables
    it { expect(subject.send(:changes)).to eql ['wow'] } # FIXME: don't access private instance variables
    it { expect(subject.send(:protocol)).to eql 'ssh' } # FIXME: don't access private instance variables
  end

  describe "#exec" do
    context "access is granted" do

      it "returns true" do
        expect(subject.exec).to be_truthy
      end
    end

    context "access is denied" do

      before do
        api.stub(check_access: GitAccessStatus.new(
                  false,
                  'denied',
                  gl_repository: nil,
                  gl_username: nil,
                  repository_path: nil,
                  gitaly: nil
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
