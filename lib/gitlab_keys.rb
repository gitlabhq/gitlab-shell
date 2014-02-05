require_relative 'gitlab_config'
require_relative 'gitlab_logger'

class GitlabKeys
  attr_accessor :auth_file, :key

  def initialize
    @command = ARGV.shift
    @key_id = ARGV.shift
    @key = ARGV.shift
    @auth_file = GitlabConfig.new.auth_file
  end

  def exec
    case @command
    when 'add-key'; add_key
    when 'rm-key';  rm_key
    when 'clear';  clear
    else
      $logger.warn "Attempt to execute invalid gitlab-keys command #{@command.inspect}."
      puts 'not allowed'
      false
    end
  end

  protected

  def add_key
    $logger.info "Adding key #{@key_id} => #{@key.inspect}"
    open('a') do |file|
      file.puts "command=\"#{ROOT_PATH}/bin/gitlab-shell #{@key_id}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty #{@key}"
    end
  end

  def rm_key
    $logger.info "Removing key #{@key_id}"
    open('r+') do |file|
      lines = []
      file.each_line do |line|
        lines << line unless line.include?("/bin/gitlab-shell #{@key_id}\"")
      end
      file.rewind
      lines.each { |line| file << line }
      file.truncate(file.pos)
    end
  end

  def clear
    open('w') do |file|
      file.puts '# Managed by gitlab-shell'
    end
  end

  def open(mode)
    File.open(auth_file, mode) do |file|
      # get an exclusive lock
      file.flock(File::LOCK_EX)
      yield file
    end
    true
  end
end
