require 'base64'
require 'json'

class GitlabLfsAuthentication
  # TODO: These don't need to be public
  attr_accessor :username, :lfs_token, :repository_http_path

  def initialize(username, lfs_token, repository_http_path, expires_in = nil)
    @username = username
    @lfs_token = lfs_token
    @repository_http_path = repository_http_path
    @expires_in = expires_in
  end

  def self.build_from_json(json)
    values = JSON.parse(json)
    new(values['username'],
        values['lfs_token'],
        values['repository_http_path'],
        values['expires_in'])
  rescue
    nil
  end

  # Source: https://github.com/git-lfs/git-lfs/blob/master/docs/api/server-discovery.md#ssh
  #
  def authentication_payload
    payload = { header: { Authorization: authorization }, href: href }
    payload[:expires_in] = @expires_in if @expires_in

    JSON.generate(payload)
  end

  private

  def authorization
    "Basic #{Base64.strict_encode64("#{username}:#{lfs_token}")}"
  end

  def href
    "#{repository_http_path}/info/lfs"
  end
end
