require 'spec_helper'
require 'gitlab_access'

describe GitlabAccess do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name)   { 'dzaporozhets/gitlab-ci' }
  let(:repo_path)  { File.join(repository_path, repo_name) + ".git" }
  let(:gitlab_access) { GitlabAccess.new(repo_path, 'key-123', '') }

  before do
    GitlabConfig.any_instance.stub(repos_path: repository_path)
  end

  describe :initialize do
    it { gitlab_access.repo_name.should == repo_name }
    it { gitlab_access.repo_path.should == repo_path }
  end
end
