require_relative 'gitlab_init'
require_relative 'gitlab_net'
require 'json'

class GitlabPostReceive
  attr_reader :config, :repo_path, :changes

  def initialize(repo_path, actor, changes)
    @config = GitlabConfig.new
    @repo_path, @actor = repo_path.strip, actor
    @changes = changes
  end

  def exec
    # reset GL_ID env since we already
    # get value from it
    ENV['GL_ID'] = nil

    update_redis

    if broadcast_message = GitlabNet.new.broadcast_message
      puts
      print_broadcast_message(broadcast_message["message"])
    end
  end

  protected

  def print_broadcast_message(message)
    # A standard terminal window is (at least) 80 characters wide.
    total_width = 80

    # Git prefixes remote messages with "remote: ", so this width is subtracted 
    # from the width available to us.
    total_width -= "remote: ".length

    # Our centered text shouldn't start or end right at the edge of the window, 
    # so we add some horizontal padding: 2 chars on either side.
    text_width = total_width - 2 * 2

    # Automatically wrap message at text_width (= 68) characters: 
    # Splits the message up into the longest possible chunks matching 
    # "<between 0 and text_width characters><space or end-of-line>".
    # The last result is always an empty string (0 chars and the end-of-line), 
    # so drop that. 
    # message.scan returns a nested array of capture groups, so flatten.
    lines = message.scan(/(.{,#{text_width}})(?:\s|$)/)[0...-1].flatten

    puts "=" * total_width
    puts

    lines.each do |line|
      line.strip!

      # Center the line by calculating the left padding measured in characters.
      line_padding = [(total_width - line.length) / 2, 0].max
      puts (" " * line_padding) + line
    end

    puts
    puts "=" * total_width
  end

  def update_redis
    queue = "#{config.redis_namespace}:queue:post_receive"
    msg = JSON.dump({'class' => 'PostReceive', 'args' => [@repo_path, @actor, @changes]})
    if system(*config.redis_command, 'rpush', queue, msg,
              err: '/dev/null', out: '/dev/null')
      return true
    else
      puts "GitLab: An unexpected error occurred (redis-cli returned #{$?.exitstatus})."
      return false
    end
  end
end
