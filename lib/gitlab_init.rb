if ENV['SHELL_ROOT_PATH'].nil? || ENV['SHELL_ROOT_PATH'].empty?
  ROOT_PATH = File.expand_path(File.join(File.dirname(__FILE__), ".."))
else
  ROOT_PATH = ENV['SHELL_ROOT_PATH']
end

require_relative 'gitlab_config'
