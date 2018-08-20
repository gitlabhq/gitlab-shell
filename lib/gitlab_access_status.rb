require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :gl_id, :gl_username, :gitaly, :git_protocol, :git_config_options

  def initialize(status, message, gl_repository: nil, gl_id: nil, gl_username: nil, gitaly: nil, git_protocol: nil, git_config_options: nil)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @gl_id = gl_id
    @gl_username = gl_username
    @git_config_options = git_config_options
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
        git_config_options: values["git_config_options"],
        gitaly: values["gitaly"],
        git_protocol: values["git_protocol"])
  end

  def allowed?
    @status
  end
end
