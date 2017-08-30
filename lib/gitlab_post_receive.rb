require_relative 'gitlab_init'
require_relative 'gitlab_net'
require_relative 'gitlab_reference_counter'
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
    @repo_path, @actor = repo_path.strip, actor
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

    response['reference_counter_decreased']
  rescue GitlabNet::ApiUnreachableError
    false
  rescue GitlabNet::NotFound
    fallback_post_receive
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
    if merge_request["new_merge_request"]
      message = "To create a merge request for #{merge_request["branch_name"]}, visit:"
    else
      message = "View merge request for #{merge_request["branch_name"]}:"
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

  def update_redis
    # Encode changes as base64 so we don't run into trouble with non-UTF-8 input.
    changes = Base64.encode64(@changes)
    # TODO: Change to `@gl_repository` in next release.
    # See https://gitlab.com/gitlab-org/gitlab-shell/merge_requests/130#note_28747613
    project_identifier = @gl_repository || @repo_path

    queue = "#{config.redis_namespace}:queue:post_receive"
    msg = JSON.dump({
      'class' => 'PostReceive',
      'args' => [project_identifier, @actor, changes],
      'jid' => @jid,
      'enqueued_at' => Time.now.to_f
    })

    begin
      GitlabNet.new.redis_client.rpush(queue, msg)
      true
    rescue => e
      $stderr.puts "GitLab: An unexpected error occurred in writing to Redis: #{e}"
      false
    end
  end

  private

  def fallback_post_receive
    result = update_redis

    begin
      broadcast_message = GitlabMetrics.measure("broadcast-message") do
        api.broadcast_message
      end

      if broadcast_message.has_key?("message")
        print_broadcast_message(broadcast_message["message"])
      end

      merge_request_urls = GitlabMetrics.measure("merge-request-urls") do
        api.merge_request_urls(@gl_repository, @repo_path, @changes)
      end
      print_merge_request_links(merge_request_urls)

      api.notify_post_receive(gl_repository, repo_path)
    rescue GitlabNet::ApiUnreachableError
      nil
    end

    result && GitlabReferenceCounter.new(repo_path).decrease
  end
end
