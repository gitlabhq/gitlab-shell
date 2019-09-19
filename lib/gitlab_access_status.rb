require 'json'

class GitAccessStatus
  HTTP_MULTIPLE_CHOICES = '300'.freeze

  attr_reader :message, :gl_repository, :gl_project_path, :gl_id, :gl_username,
              :gl_type, :gitaly, :git_protocol, :git_config_options, :payload,
              :gl_console_messages

  def initialize(status, status_code, message, gl_repository: nil,
                 gl_project_path: nil, gl_id: nil,
                 gl_username: nil, gl_type: nil, gitaly: nil, git_protocol: nil,
                 git_config_options: nil, payload: nil, gl_console_messages: [])
    @status = status
    @status_code = status_code
    @message = message
    @gl_repository = gl_repository
    @gl_project_path = gl_project_path
    @gl_id = gl_id
    @gl_username = gl_username
    @gl_type = gl_type
    @git_config_options = git_config_options
    @gitaly = gitaly
    @git_protocol = git_protocol
    @payload = payload
    @gl_console_messages = gl_console_messages
  end

  def self.create_from_json(json, status_code)
    values = JSON.parse(json)
    new(values["status"],
        status_code,
        values["message"],
        gl_repository: values["gl_repository"],
        gl_project_path: values["gl_project_path"],
        gl_id: values["gl_id"],
        gl_username: values["gl_username"],
        gl_type: values["gl_type"],
        git_config_options: values["git_config_options"],
        gitaly: values["gitaly"],
        git_protocol: values["git_protocol"],
        payload: values["payload"],
        gl_console_messages: values["gl_console_messages"])
  end

  def allowed?
    @status
  end

  def custom_action?
    @status_code == HTTP_MULTIPLE_CHOICES
  end

  def success?
    @status == true
  end
end
