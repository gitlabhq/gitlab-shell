require_relative 'base'

module Actor
  class User < Base
    alias username identifier

    def self.identifier_prefix
      'user'.freeze
    end

    def self.identifier_key
      'user_id'.freeze
    end

    def self.id_regex
      /\Auser\-\d+\Z/
    end
  end
end
