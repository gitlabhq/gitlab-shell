require_relative 'gitlab_init'

class GitlabMergeRequestHook
  attr_reader :config, :repo_path, :changes

  def initialize(repo_path, config, changes)
    @branch    = branch_name(changes)
    @config    = config
    @repo_path = repo_path
  end

  def exec
    return if @branch.nil? || @branch == 'master'
    puts
    puts "To open a merge request for #{@branch}, enter in:"
    puts "\t#{repo_url}"
    puts
  end

  private

  def branch_name(changes)
    changes.
      encode('UTF-8', 'binary', invalid: :replace, replace: '').
      split.
      select { |change| change.include?('refs/head') }.
      last.gsub!('refs/heads/', '') rescue nil
  end

  def repo_url
    repo_url = @repo_path.split('/').last(2).join('/')
    repo_url.gsub!(/\.git\z/, '')
    "#{@config.gitlab_url}/#{repo_url}/merge_requests/new?" \
      "merge_request[source_branch]=#{@branch}&merge_request[target_branch]=master"
  end

end
