require 'json'

class GitAccessStatus
  attr_reader :message, :repository_path

  def initialize(status, message, repository_path)
    @status = status
    @message = message
    @repository_path = repository_path
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"], values["repository_path"])
  end

  def allowed?
    @status
  end
end
