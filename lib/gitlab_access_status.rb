require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :gl_username, :repository_path, :gitaly

  def initialize(status, message, gl_repository:, gl_username:, repository_path:, gitaly:)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @gl_username = gl_username
    @repository_path = repository_path
    @gitaly = gitaly
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    new(values["status"],
        values["message"],
        gl_repository: values["gl_repository"],
        gl_username: values["gl_username"],
        repository_path: values["repository_path"],
        gitaly: values["gitaly"])
  end

  def allowed?
    @status
  end
end
