require 'spec_helper'
require 'gitlab_access'

describe GitlabAccess do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name)   { 'dzaporozhets/gitlab-ci' }
  let(:api) do
    double(GitlabNet).tap do |api|
      api.stub(check_access: GitAccessStatus.new(true,
                                                 'ok',
                                                 gl_repository: 'project-1',
                                                 gl_username: 'testuser',
                                                 gitaly: nil))
    end
  end
  subject do
    GitlabAccess.new(nil, 'key-123', 'wow', 'ssh').tap do |access|
      access.stub(exec_cmd: :exec_called)
      access.stub(api: api)
    end
  end

  describe :initialize do
    it { subject.changes.should == ['wow'] }
    it { subject.protocol.should == 'ssh' }
  end

  describe "#exec" do
    context "access is granted" do

      it "returns true" do
        expect(subject.exec).to be_true
      end
    end

    context "access is denied" do

      before do
        api.stub(check_access: GitAccessStatus.new(
                  false,
                  'denied',
                  gl_repository: nil,
                  gl_username: nil,
                  gitaly: nil
                ))
      end

      it "returns false" do
        expect(subject.exec).to be_false
      end
    end

    context "API connection fails" do

      before do
        api.stub(:check_access).and_raise(GitlabNet::ApiUnreachableError)
      end

      it "returns false" do
        expect(subject.exec).to be_false
      end
    end
  end
end
