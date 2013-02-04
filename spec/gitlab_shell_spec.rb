require_relative '../lib/gitlab_shell'

describe GitlabShell do

  describe :initialize do
    before do
      ROOT_PATH = File.join(File.expand_path(File.dirname(__FILE__)), "..")
      ENV['SSH_ORIGINAL_COMMAND'] = 'git-receive-pack'
      ARGV = ['dzaporozhets']
      @shell = GitlabShell.new
    end

    it { @shell.username.should == 'dzaporozhets' }
    it { @shell.repos_path.should == "/home/git/repositories" }
  end
end
