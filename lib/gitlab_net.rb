require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_access'
require_relative 'gitlab_lfs_authentication'
require_relative 'http_helper'

class GitlabNet # rubocop:disable Metrics/ClassLength
  include HTTPHelper

  CHECK_TIMEOUT = 5
  API_INACCESSIBLE_MESSAGE = 'API is not accessible'.freeze

  def check_access(cmd, gl_repository, repo, who, changes, protocol, env: {})
    changes = changes.join("\n") unless changes.is_a?(String)

    params = {
      action: cmd,
      changes: changes,
      gl_repository: gl_repository,
      project: sanitize_path(repo),
      protocol: protocol,
      env: env
    }

    who_sym, _, who_v = self.class.parse_who(who)
    params[who_sym] = who_v

    url = "#{internal_api_endpoint}/allowed"
    resp = post(url, params)

    case resp
    when Net::HTTPSuccess, Net::HTTPMultipleChoices, Net::HTTPUnauthorized,
         Net::HTTPNotFound, Net::HTTPServiceUnavailable
      if resp.content_type == CONTENT_TYPE_JSON
        return GitAccessStatus.create_from_json(resp.body, resp.code)
      end
    end

    GitAccessStatus.new(false, resp.code, API_INACCESSIBLE_MESSAGE)
  end

  def discover(who)
    _, who_k, who_v = self.class.parse_who(who)

    resp = get("#{internal_api_endpoint}/discover?#{who_k}=#{who_v}")

    JSON.parse(resp.body) rescue nil
  end

  def lfs_authenticate(gl_id, repo, operation)
    id_sym, _, id = self.class.parse_who(gl_id)
    params = { project: sanitize_path(repo), operation: operation }

    case id_sym
    when :key_id
      params[:key_id] = id
    when :user_id
      params[:user_id] = id
    else
      raise ArgumentError, "lfs_authenticate() got unsupported GL_ID='#{gl_id}'!"
    end

    resp = post("#{internal_api_endpoint}/lfs_authenticate", params)

    GitlabLfsAuthentication.build_from_json(resp.body) if resp.code == '200'
  end

  def broadcast_message
    resp = get("#{internal_api_endpoint}/broadcast_message")
    JSON.parse(resp.body) rescue {}
  end

  def merge_request_urls(gl_repository, repo_path, changes)
    changes = changes.join("\n") unless changes.is_a?(String)
    changes = changes.encode('UTF-8', 'ASCII', invalid: :replace, replace: '')
    url = "#{internal_api_endpoint}/merge_request_urls?project=#{URI.escape(repo_path)}&changes=#{URI.escape(changes)}"
    url += "&gl_repository=#{URI.escape(gl_repository)}" if gl_repository
    resp = get(url)

    if resp.code == '200'
      JSON.parse(resp.body)
    else
      []
    end
  rescue
    []
  end

  def check
    get("#{internal_api_endpoint}/check", options: { read_timeout: CHECK_TIMEOUT })
  end

  def authorized_key(key)
    resp = get("#{internal_api_endpoint}/authorized_keys?key=#{URI.escape(key, '+/=')}")
    JSON.parse(resp.body) if resp.code == "200"
  rescue
    nil
  end

  def two_factor_recovery_codes(gl_id)
    id_sym, _, id = self.class.parse_who(gl_id)

    resp = post("#{internal_api_endpoint}/two_factor_recovery_codes", id_sym => id)

    JSON.parse(resp.body) if resp.code == '200'
  rescue
    {}
  end

  def notify_post_receive(gl_repository, repo_path)
    params = { gl_repository: gl_repository, project: repo_path }
    resp = post("#{internal_api_endpoint}/notify_post_receive", params)

    resp.code == '200'
  rescue
    false
  end

  def post_receive(gl_repository, identifier, changes)
    params = {
      gl_repository: gl_repository,
      identifier: identifier,
      changes: changes
    }
    resp = post("#{internal_api_endpoint}/post_receive", params)

    raise NotFound if resp.code == '404'

    JSON.parse(resp.body) if resp.code == '200'
  end

  def pre_receive(gl_repository)
    resp = post("#{internal_api_endpoint}/pre_receive", gl_repository: gl_repository)

    raise NotFound if resp.code == '404'

    JSON.parse(resp.body) if resp.code == '200'
  end

  def self.parse_who(who)
    if who.start_with?("key-")
      value = who.gsub("key-", "")
      raise ArgumentError, "who='#{who}' is invalid!" unless value =~ /\A[0-9]+\z/
      [:key_id, 'key_id', value]
    elsif who.start_with?("user-")
      value = who.gsub("user-", "")
      raise ArgumentError, "who='#{who}' is invalid!" unless value =~ /\A[0-9]+\z/
      [:user_id, 'user_id', value]
    elsif who.start_with?("username-")
      [:username, 'username', who.gsub("username-", "")]
    else
      raise ArgumentError, "who='#{who}' is invalid!"
    end
  end

  protected

  def sanitize_path(repo)
    repo.delete("'")
  end
end
