require 'json'

require_relative '../gitlab_config'
require_relative '../gitlab_net'
require_relative '../gitlab_metrics'
require_relative '../user'

module Action
  class Base
    def self.create_from_json(_)
      raise NotImplementedError
    end

    private

    attr_reader :key_id

    def config
      @config ||= GitlabConfig.new
    end

    def api
      @api ||= GitlabNet.new
    end

    def user
      @user ||= User.new(key_id, audit_usernames: config.audit_usernames)
    end
  end
end
