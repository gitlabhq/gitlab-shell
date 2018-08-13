require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :gl_id, :gl_username, :repository_path, :gitaly, :git_protocol

  def initialize(status, message, gl_repository:, gl_id:, gl_username:, repository_path:, gitaly:, git_protocol:)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @gl_id = gl_id
    @gl_username = gl_username
    @repository_path = repository_path
    @gitaly = gitaly
    @git_protocol = git_protocol
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    new(values["status"],
        values["message"],
        gl_repository: values["gl_repository"],
        gl_id: values["gl_id"],
        gl_username: values["gl_username"],
        repository_path: values["repository_path"],
        gitaly: values["gitaly"],
        git_protocol: values["git_protocol"])
  end

  def allowed?
    @status
  end
end
