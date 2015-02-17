require 'json'

class GitAccessStatus
  attr_accessor :status, :message
  alias_method :allowed?, :status

  def initialize(status, message = '')
    @status = status
    @message = message
  end

  def self.create_from_json(json)
    values = JSON.parse(json)
    self.new(values["status"], values["message"])
  end

  def to_json
    { status: @status, message: @message }.to_json
  end
end
