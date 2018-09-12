require 'json'
require 'logger'
require 'time'

require_relative 'gitlab_config'

def convert_log_level(log_level)
  Logger.const_get(log_level.upcase)
rescue NameError
  $stderr.puts "WARNING: Unrecognized log level #{log_level.inspect}."
  $stderr.puts "WARNING: Falling back to INFO."
  Logger::INFO
end

class GitlabLogger
  # Emulate the quoting logic of logrus
  # https://github.com/sirupsen/logrus/blob/v1.0.5/text_formatter.go#L143-L156
  SHOULD_QUOTE = /[^a-zA-Z0-9\-._\/@^+]/

  LEVELS = {
    Logger::INFO => 'info'.freeze,
    Logger::DEBUG => 'debug'.freeze,
    Logger::WARN => 'warn'.freeze,
    Logger::ERROR => 'error'.freeze
  }.freeze

  def initialize(level, path, log_format)
    @level = level

    @log_file = File.open(path, 'ab')
    # By default Ruby will buffer writes. This is a problem when we exec
    # into a new command before Ruby flushed its buffers. Setting 'sync' to
    # true disables Ruby's buffering.
    @log_file.sync = true

    @log_format = log_format
  end

  def info(message, data = {})
    log_at(Logger::INFO, message, data)
  end

  def debug(message, data = {})
    log_at(Logger::DEBUG, message, data)
  end

  def warn(message, data = {})
    log_at(Logger::WARN, message, data)
  end

  def error(message, data = {})
    log_at(Logger::ERROR, message, data)
  end

  private

  attr_reader :log_file, :log_format

  def log_at(level, message, data)
    return unless @level <= level

    data[:pid] = pid
    data[:level] = LEVELS[level]
    data[:msg] = message

    # Use RFC3339 to match logrus in the Go parts of gitlab-shell
    data[:time] = time_now.to_datetime.rfc3339

    case log_format
    when 'json'
      # Don't use IO#puts because of https://bugs.ruby-lang.org/issues/14042
      log_file.print("#{format_json(data)}\n")
    else
      log_file.print("#{format_text(data)}\n")
    end
  end

  def pid
    Process.pid
  end

  def time_now
    Time.now
  end

  def format_text(data)
    # We start the line with these fields to match the behavior of logrus
    result = [
      format_key_value(:time, data.delete(:time)),
      format_key_value(:level, data.delete(:level)),
      format_key_value(:msg, data.delete(:msg))
    ]

    data.sort.each { |k, v| result << format_key_value(k, v) }
    result.join(' ')
  end

  def format_key_value(key, value)
    value_string = value.to_s
    value_string = value_string.inspect if SHOULD_QUOTE =~ value_string

    "#{key}=#{value_string}"
  end

  def format_json(data)
    data.each do |key, value|
      next unless value.is_a?(String)

      value = value.dup.force_encoding('utf-8')
      value = value.inspect unless value.valid_encoding?
      data[key] = value.freeze
    end

    data.to_json
  end
end

config = GitlabConfig.new

$logger = GitlabLogger.new(convert_log_level(config.log_level), config.log_file, config.log_format)
