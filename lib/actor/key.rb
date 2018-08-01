require_relative 'base'
require_relative '../gitlab_net'

module Actor
  class Key < Base
    ANONYMOUS_USER = 'Anonymous'.freeze

    alias key_id id

    def self.identifier_prefix
      'key'.freeze
    end

    def self.identifier_key
      'key_id'.freeze
    end

    def self.id_regex
      /\Akey\-\d+\Z/
    end

    def username
      @username ||= begin
        user = GitlabNet.new.discover(self)
        user ? "@#{user['username']}" : ANONYMOUS_USER
      end
    end
  end
end
