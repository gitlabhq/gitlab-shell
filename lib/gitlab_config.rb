require 'yaml'

class GitlabConfig
  attr_reader :config

  def initialize
    @config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
  end

  def repos_path
    @config['repos_path'] = "/home/git/repositories"
  end

  def auth_file
    @config['auth_file'] = "/home/git/.ssh/authorized_keys"
  end
end
