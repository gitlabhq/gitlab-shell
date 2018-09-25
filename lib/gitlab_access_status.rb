require 'json'

class GitAccessStatus
  HTTP_MULTIPLE_CHOICES = '300'.freeze

  attr_reader :message, :gl_repository, :gl_id, :gl_username, :gitaly, :git_protocol, :git_config_options, :payload

  def initialize(status, status_code, message, gl_repository: nil, gl_id: nil,
                 gl_username: nil, gitaly: nil, git_protocol: nil,
                 git_config_options: nil, payload: nil)
    @status = status
    @status_code = status_code
    @message = message
    @gl_repository = gl_repository
    @gl_id = gl_id
    @gl_username = gl_username
    @git_config_options = git_config_options
    @gitaly = gitaly
    @git_protocol = git_protocol
    @payload = payload
  end

  def self.create_from_json(json, status_code)
    values = JSON.parse(json)
    new(values["status"],
        status_code,
        values["message"],
        gl_repository: values["gl_repository"],
        gl_id: values["gl_id"],
        gl_username: values["gl_username"],
        git_config_options: values["git_config_options"],
        gitaly: values["gitaly"],
        git_protocol: values["git_protocol"],
        payload: values["payload"])
  end

  def allowed?
    @status
  end

  def custom_action?
    @status_code == HTTP_MULTIPLE_CHOICES
  end
end
