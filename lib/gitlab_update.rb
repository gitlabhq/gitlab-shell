require_relative 'gitlab_init'
require_relative 'gitlab_net'

class GitlabUpdate
  def initialize(repo_path, key_id, refname)
    @repo_name = repo_path
    @repo_name.gsub!(GitlabConfig.new.repos_path.to_s, "")
    @repo_name.gsub!(/.git$/, "")
    @repo_name.gsub!(/^\//, "")

    @key_id = key_id
    @refname = /refs\/heads\/([\w\.-]+)/.match(refname).to_a.last
  end

  def exec
    # Skip update hook for local push when key_id is nil
    # It required for gitlab instance to make local pushes
    # without validation of access
    exit 0 if @key_id.nil?

    if api.allowed?('git-receive-pack', @repo_name, @key_id, @refname)
      exit 0
    else
      puts "GitLab: You are not allowed to access #{@refname}! "
      exit 1
    end
  end

  protected

  def api
    GitlabNet.new
  end
end
