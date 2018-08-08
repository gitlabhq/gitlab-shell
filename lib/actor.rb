require_relative 'actor/base'
require_relative 'actor/key'
require_relative 'actor/user'
require_relative 'actor/username'

module Actor
  class UnsupportedActorError < StandardError; end

  def self.new_from(str, audit_usernames: false)
    case str
    when Key.id_regex
      Key.from(str, audit_usernames: audit_usernames)
    when User.id_regex
      User.from(str, audit_usernames: audit_usernames)
    when Username.id_regex
      Username.from(str, audit_usernames: audit_usernames)
    else
      raise UnsupportedActorError
    end
  end
end
