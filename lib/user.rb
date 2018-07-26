require_relative 'gitlab_net'

class User
  ANONYMOUS_USER = 'Anonymous'.freeze

  def initialize(key_id, audit_usernames: false)
    @key_id = key_id
    @audit_usernames = audit_usernames
  end

  def username
    @username ||= begin
      user = GitlabNet.new.discover(key_id)
      user ? "@#{user['username']}" : ANONYMOUS_USER
    end
  end

  def log_username
    audit_usernames ? username : "user with key #{key_id}"
  end

  private

  attr_reader :key_id, :audit_usernames
end
