require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_metrics'
require 'json'
require 'base64'
require 'securerandom'

class GitlabPostReceive
  include NamesHelper

  attr_reader :config, :gl_repository, :repo_path, :changes, :jid

  def initialize(gl_repository, repo_path, actor, changes)
    @config = GitlabConfig.new
    @gl_repository = gl_repository
    @repo_path = repo_path.strip
    @actor = actor
    @changes = changes
    @jid = SecureRandom.hex(12)
  end

  def exec
    response = GitlabMetrics.measure("post-receive") do
      api.post_receive(gl_repository, @actor, changes)
    end

    return false unless response

    print_broadcast_message(response['broadcast_message']) if response['broadcast_message']
    print_merge_request_links(response['merge_request_urls']) if response['merge_request_urls']
    puts response['redirected_message'] if response['redirected_message']
    puts response['project_created_message'] if response['project_created_message']

    response['reference_counter_decreased']
  rescue GitlabNet::ApiUnreachableError
    false
  end

  protected

  def api
    @api ||= GitlabNet.new
  end

  def print_merge_request_links(merge_request_urls)
    return if merge_request_urls.empty?

    puts
    merge_request_urls.each { |mr| print_merge_request_link(mr) }
  end

  def print_merge_request_link(merge_request)
    message =
      if merge_request["new_merge_request"]
        "To create a merge request for #{merge_request['branch_name']}, visit:"
      else
        "View merge request for #{merge_request['branch_name']}:"
      end

    puts message
    puts((" " * 2) + merge_request["url"])
    puts
  end

  def print_broadcast_message(message)
    # A standard terminal window is (at least) 80 characters wide.
    total_width = 80

    # Git prefixes remote messages with "remote: ", so this width is subtracted
    # from the width available to us.
    total_width -= "remote: ".length # rubocop:disable Performance/FixedSize

    # Our centered text shouldn't start or end right at the edge of the window,
    # so we add some horizontal padding: 2 chars on either side.
    text_width = total_width - 2 * 2

    # Automatically wrap message at text_width (= 68) characters:
    # Splits the message up into the longest possible chunks matching
    # "<between 0 and text_width characters><space or end-of-line>".

    msg_start_idx = 0
    lines = []
    while msg_start_idx < message.length
      parsed_line = parse_broadcast_msg(message[msg_start_idx..-1], text_width)
      msg_start_idx += parsed_line.length
      lines.push(parsed_line.strip)
    end

    puts
    puts "=" * total_width
    puts

    lines.each do |line|
      line.strip!

      # Center the line by calculating the left padding measured in characters.
      line_padding = [(total_width - line.length) / 2, 0].max
      puts((" " * line_padding) + line)
    end

    puts
    puts "=" * total_width
  end

  private

  def parse_broadcast_msg(msg, text_length)
    msg ||= ""
    # just return msg if shorter than or equal to text length
    return msg if msg.length <= text_length

    # search for word break shorter than text length
    truncate_to_space = msg.match(/\A(.{,#{text_length}})(?=\s|$)(\s*)/).to_s

    if truncate_to_space.empty?
      # search for word break longer than text length
      truncate_to_space = msg.match(/\A\S+/).to_s
    end

    truncate_to_space
  end
end
