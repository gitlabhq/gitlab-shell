require_relative 'base'

module Actor
  class Username < Base
    alias username identifier

    def self.identifier_prefix
      'username'.freeze
    end

    def self.identifier_key
      'username'.freeze
    end

    def self.id_regex
      /\Ausername\-\d+\Z/
    end
  end
end
