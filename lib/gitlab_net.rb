require 'net/http'
require 'json'

require_relative 'gitlab_config'

class GitlabNet
  def allowed?(cmd, repo, key, ref)
    project_name = repo.gsub("'", "")
    project_name = project_name.gsub(/\.git\Z/, "")
    project_name = project_name.gsub(/\A\//, "")

    key_id = key.gsub("key-", "")

    url = "#{host}/allowed?key_id=#{key_id}&action=#{cmd}&ref=#{ref}&project=#{project_name}"

    resp = get(url)

    !!(resp.code == '200' && resp.body == 'true')
  end

  def discover(key)
    key_id = key.gsub("key-", "")
    resp = get("#{host}/discover?key_id=#{key_id}")
    JSON.parse(resp.body)
  end

  def check
    get("#{host}/check")
  end

  protected

  def host
    "#{GitlabConfig.new.gitlab_url}api/v3/internal"
  end

  def get(url)
    url = URI.parse(url)
    http = Net::HTTP.new(url.host, url.port)
    http.use_ssl = (url.port == 443)
    request = Net::HTTP::Get.new(url.request_uri)
    http.start {|http| http.request(request) }
  end
end
