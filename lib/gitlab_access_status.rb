require 'json'

class GitAccessStatus
  attr_reader :message

  def initialize(status, message)
    @status = status
    @message = message
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"])
  end

  def allowed?
    @status
  end
end
