require 'yaml'

class GitlabConfig
  attr_reader :config

  def initialize
    @config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
  end

  def auth_file
    @config['auth_file'] ||= File.join(Dir.home, '.ssh/authorized_keys')
  end
end
