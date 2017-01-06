require_relative 'http_client'

class GitalyClient < HttpClient
  attr_reader :gitaly_url

  def initialize(gitaly_url)
    @gitaly_url = gitaly_url
  end

  def notify_post_receive(repo_path)
    url = "#{gitaly_url}/post-receive"
    params = { project: sanitize_path(repo_path) }

    # NOTE: Consider being more permisive with gitaly once it's more robust
    resp = post(url, params, read_timeout: 5)

    resp.code == '200'
  end
end
