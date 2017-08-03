require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :gl_username, :repository_path, :gitaly, :geo_node

  def initialize(status, message, gl_repository:, gl_username:, repository_path:, gitaly:, geo_node:)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @gl_username = gl_username
    @repository_path = repository_path
    @gitaly = gitaly
    @geo_node = geo_node
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"],
             values["message"],
             gl_repository: values["gl_repository"],
             gl_username: values["gl_username"],
             repository_path: values["repository_path"],
             gitaly: values["gitaly"],
             geo_node: values["geo_node"])
  end

  def allowed?
    @status
  end
end
