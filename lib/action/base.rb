require 'json'

require_relative '../gitlab_config'
require_relative '../gitlab_net'
require_relative '../gitlab_metrics'

module Action
  class Base
    def initialize
      raise NotImplementedError
    end

    def self.create_from_json(_)
      raise NotImplementedError
    end

    private

    def config
      @config ||= GitlabConfig.new
    end

    def api
      @api ||= GitlabNet.new
    end
  end
end
