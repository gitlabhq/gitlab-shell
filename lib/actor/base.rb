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

    def log_username
      audit_usernames? ? username : "#{klass_name.downcase} with identifier #{identifier}"
    end

    private

    attr_reader :audit_usernames

    def klass_name
      self.class.to_s.split('::')[-1]
    end

    def audit_usernames?
      audit_usernames
    end
  end
end
