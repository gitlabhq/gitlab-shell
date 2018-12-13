require 'base64'
require 'json'

class GitlabLfsAuthentication
  attr_accessor :username, :lfs_token, :repository_http_path

  def initialize(username, lfs_token, repository_http_path)
    @username = username
    @lfs_token = lfs_token
    @repository_http_path = repository_http_path
  end

  def self.build_from_json(json)
    values = JSON.parse(json)
    new(values['username'], values['lfs_token'], values['repository_http_path'])
  rescue StandardError
    nil
  end

  def authentication_payload
    authorization = {
      header: {
        Authorization: "Basic #{Base64.strict_encode64("#{username}:#{lfs_token}")}"
      },
      href: "#{repository_http_path}/info/lfs/"
    }

    JSON.generate(authorization)
  end
end
