require_relative 'spec_helper'
require_relative '../lib/gitlab_projects'

describe GitlabProjects do
  describe :initialize do
    before do
      argv('add-project', 'gitlab-ci.git')
      @gl_projects = GitlabProjects.new
    end

    it { @gl_projects.project_name.should == 'gitlab-ci.git' }
    it { @gl_projects.instance_variable_get(:@command).should == 'add-project' }
    it { @gl_projects.instance_variable_get(:@full_path).should == '/home/git/repositories/gitlab-ci.git' }
  end

  describe :add_project do
    before do
      argv('add-project', 'gitlab-ci.git')
      @gl_projects = GitlabProjects.new
    end

    it "should receive valid cmd" do
      valid_cmd = "cd /home/git/repositories/gitlab-ci.git && git init --bare && ln -s /home/git/gitlab-shell/hooks/post-receive /home/git/repositories/gitlab-ci.git/hooks/post-receive"
      @gl_projects.should_receive(:system).with(valid_cmd)
      @gl_projects.send :add_project
    end
  end

  def argv(*args)
    args.each_with_index do |arg, i|
      ARGV[i] = arg
    end
  end
end
