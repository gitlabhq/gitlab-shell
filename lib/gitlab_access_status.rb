require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :repository_path, :gitaly, :geo_node

  def initialize(status, message, gl_repository, repository_path, gitaly, geo_node)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @repository_path = repository_path
    @gitaly = gitaly
    @geo_node = geo_node
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"],
             values["message"],
             values["gl_repository"],
             values["repository_path"],
             values["gitaly"],
             values["geo_node"])
  end

  def allowed?
    @status
  end
end
