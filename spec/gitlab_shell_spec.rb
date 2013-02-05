require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'

describe GitlabShell do

  describe :initialize do
    before do
      ENV['SSH_ORIGINAL_COMMAND'] = 'git-receive-pack'
      ARGV[0] = 'dzaporozhets'
      @shell = GitlabShell.new
    end

    it { @shell.username.should == 'dzaporozhets' }
    it { @shell.repos_path.should == "/home/git/repositories" }
  end
end
