require 'json'

class GitAccessStatus
  attr_reader :message, :repository_path, :gitaly_address

  def initialize(status, message, repository_path, gitaly_address = nil)
    @status = status
    @message = message
    @repository_path = repository_path
    @gitaly_address = gitaly_address
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"], values["repository_path"], values["gitaly_address"])
  end

  def allowed?
    @status
  end
end
