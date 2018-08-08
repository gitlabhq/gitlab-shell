module Actor
  class Base
    attr_reader :id

    def initialize(id, audit_usernames: false)
      @id = id
      @audit_usernames = audit_usernames
    end

    def self.from(str, audit_usernames: false)
      new(str.gsub(/#{identifier_prefix}-/, ''), audit_usernames: audit_usernames)
    end

    def self.identifier_key
      raise NotImplementedError
    end

    def self.identifier_prefix
      raise NotImplementedError
    end

    def self.id_regex
      raise NotImplementedError
    end

    def username
      raise NotImplementedError
    end

    def identifier
      "#{self.class.identifier_prefix}-#{id}"
    end

    def identifier_key
      self.class.identifier_key
    end

    def log_username
      audit_usernames? ? username : "#{label} with identifier #{identifier}"
    end

    private

    attr_reader :audit_usernames

    alias audit_usernames? audit_usernames

    def klass_name
      self.class.to_s.split('::')[-1]
    end

    def label
      klass_name.downcase
    end
  end
end
