require 'logger'

require_relative 'gitlab_config'

def convert_log_level(log_level)
  Logger.const_get(log_level.upcase)
rescue NameError
  $stderr.puts "WARNING: Unrecognized log level #{log_level.inspect}."
  $stderr.puts "WARNING: Falling back to INFO."
  Logger::INFO
end

config = GitlabConfig.new

$logger = Logger.new(config.log_file)
$logger.level = convert_log_level(config.log_level)
