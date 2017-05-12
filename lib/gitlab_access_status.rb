require 'json'

class GitAccessStatus
  attr_reader :message, :gl_repository, :repository_path

  def initialize(status, message, gl_repository, repository_path)
    @status = status
    @message = message
    @gl_repository = gl_repository
    @repository_path = repository_path
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"], values["gl_repository"], values["repository_path"])
  end

  def allowed?
    @status
  end
end
