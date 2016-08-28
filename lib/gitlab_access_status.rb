require 'json'

class GitAccessStatus
  attr_reader :message, :repository_path, :memory_status, :memory_message

  def initialize(status, message, repository_path, memory = nil)
    @status = status
    @message = message
    @repository_path = repository_path

    if memory
      @memory_status = memory["status"]
      @memory_message = memory["message"]
    end
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"],
             values["repository_path"], values["memory"])
  end

  def allowed?
    @status
  end

  def memory_limit?
    @memory_status
  end
end
