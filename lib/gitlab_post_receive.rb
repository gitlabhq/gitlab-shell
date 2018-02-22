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
    # The last result is always an empty string (0 chars and the end-of-line),
    # so drop that.
    # message.scan returns a nested array of capture groups, so flatten.
    lines = message.scan(/(.{,#{text_width}})(?:\s|$)/)[0...-1].flatten

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
end
