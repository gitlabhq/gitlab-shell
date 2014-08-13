require 'spec_helper'
require 'gitlab_update'

describe GitlabUpdate do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name)   { 'dzaporozhets/gitlab-ci' }
  let(:repo_path)  { File.join(repository_path, repo_name) + ".git" }
  let(:ref) { 'refs/heads/awesome-feature' }
  let(:gitlab_update) { GitlabUpdate.new(repo_path, 'key-123', ref) }

  before do
    ARGV[1] = 'd1e3ca3b25'
    ARGV[2] = 'c2b3653b25'
    GitlabConfig.any_instance.stub(repos_path: repository_path)
  end

  describe :initialize do
    it { gitlab_update.repo_name.should == repo_name }
    it { gitlab_update.repo_path.should == repo_path }
    it { gitlab_update.ref.should == ref }
    it { gitlab_update.ref_name.should == 'awesome-feature' }
    it { gitlab_update.oldrev.should == 'd1e3ca3b25' }
    it { gitlab_update.newrev.should == 'c2b3653b25' }
  end
end
