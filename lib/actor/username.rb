require_relative 'base'
require_relative 'key'

module Actor
  class Username < Key
    def self.identifier_prefix
      'username'.freeze
    end

    def self.identifier_key
      'username'.freeze
    end

    def self.id_regex
      /\Ausername\-[a-z0-9-]+\z/
    end

    private

    # Override Base#label
    def label
      'user'
    end
  end
end
