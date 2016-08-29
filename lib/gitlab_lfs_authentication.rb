require 'base64'
require 'json'

class GitlabLfsAuthentication
  attr_accessor :user, :repository_http_path

  def initialize(user, repository_http_path)
    @user = user
    @repository_http_path = repository_http_path
  end

  def authenticate!
    authorization = {
      header: {
        Authorization: "Basic #{Base64.strict_encode64("#{user['username']}:#{user['lfs_token']}")}"
      },
      href: "#{repository_http_path}/info/lfs/"
    }

    JSON.generate(authorization)
  end
end
