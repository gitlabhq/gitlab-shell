require_relative 'spec_helper'
require_relative '../lib/gitlab_update'

describe GitlabUpdate do
  describe :exec do
    let(:repo_name)   { 'myproject' }
    let(:key_id)      { 'key-123' }
    let(:branch_name) { '/refs/heads/master' }
    let(:api) { double(GitlabNet) }
    subject do
      described_class.new(repo_name, key_id, branch_name).tap do |o|
        o.stub(:api => api)
      end
    end

    context "when the user is not allowed to push to a protected branch" do
      before { api.stub(:allowed? => false) }
      it "should deny pushing" do
        expect { subject.exec }.to terminate.with_code(1)
      end
    end

    context "when the user is allowed to push to a protected branch" do
      before { api.stub(:allowed? => true) }
      it "should allow pushing" do
        expect { subject.exec }.to terminate
      end
    end

    context "when the branch name contains a slash" do
      let(:branch_name) { '/refs/heads/users/bob/master' }
      it "should check the correct branch name" do
        api.should_receive(:allowed?).with('git-receive-pack', repo_name, key_id, 'users/bob/master').and_return(true)
        expect { subject.exec }.to terminate
      end
    end
  end
end