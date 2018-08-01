require 'json'

require_relative 'errors'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'
require_relative 'gitlab_lfs_authentication'
require_relative 'http_helper'
require_relative 'action'

class GitlabNet
  include HTTPHelper

  CHECK_TIMEOUT = 5
  GL_PROTOCOL = 'ssh'.freeze
  API_INACCESSIBLE_ERROR = 'API is not accessible'.freeze

  def check_access(cmd, gl_repository, repo, actor, changes, protocol = GL_PROTOCOL, env: {})
    changes = changes.join("\n") unless changes.is_a?(String)

    params = {
      action: cmd,
      changes: changes,
      gl_repository: gl_repository,
      project: sanitize_path(repo),
      protocol: protocol,
      env: env
    }

    params[actor.identifier_key.to_sym] = actor.id

    resp = post("#{internal_api_endpoint}/allowed", params)

    determine_action(actor, resp)
  end

  def discover(actor)
    resp = get("#{internal_api_endpoint}/discover?#{actor.identifier_key}=#{actor.id}")
    JSON.parse(resp.body)
  rescue JSON::ParserError, ApiUnreachableError
    nil
  end

  def lfs_authenticate(actor, repo)
    params = { project: sanitize_path(repo) }
    params[actor.identifier_key.to_sym] = actor.id

    resp = post("#{internal_api_endpoint}/lfs_authenticate", params)

    GitlabLfsAuthentication.build_from_json(resp.body) if resp.code == HTTP_SUCCESS
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

    resp.code == HTTP_SUCCESS ? JSON.parse(resp.body) : []
  rescue
    []
  end

  def check
    get("#{internal_api_endpoint}/check", options: { read_timeout: CHECK_TIMEOUT })
  end

  def authorized_key(full_key)
    resp = get("#{internal_api_endpoint}/authorized_keys?key=#{URI.escape(full_key, '+/=')}")
    JSON.parse(resp.body) if resp.code == HTTP_SUCCESS
  rescue
    nil
  end

  def two_factor_recovery_codes(actor)
    params = { actor.identifier_key.to_sym => actor.id }
    resp = post("#{internal_api_endpoint}/two_factor_recovery_codes", params)
    JSON.parse(resp.body) if resp.code == HTTP_SUCCESS
  rescue
    {}
  end

  def notify_post_receive(gl_repository, repo_path)
    params = { gl_repository: gl_repository, project: repo_path }
    resp = post("#{internal_api_endpoint}/notify_post_receive", params)

    resp.code == HTTP_SUCCESS
  rescue
    false
  end

  def post_receive(gl_repository, actor, changes)
    params = { gl_repository: gl_repository, identifier: actor.identifier, changes: changes }
    resp = post("#{internal_api_endpoint}/post_receive", params)
    raise NotFoundError if resp.code == HTTP_NOT_FOUND

    JSON.parse(resp.body) if resp.code == HTTP_SUCCESS
  end

  def pre_receive(gl_repository)
    resp = post("#{internal_api_endpoint}/pre_receive", gl_repository: gl_repository)
    raise NotFoundError if resp.code == HTTP_NOT_FOUND

    JSON.parse(resp.body) if resp.code == HTTP_SUCCESS
  end

  private

  def sanitize_path(repo)
    repo.delete("'")
  end

  def determine_action(actor, resp)
    json = JSON.parse(resp.body)
    message = json['message']

    case resp.code
    when HTTP_SUCCESS
      # TODO: This raise can be removed once internal API can respond with correct
      #       HTTP status codes, instead of relying upon parsing the body and
      #       accessing the 'status' key.
      raise AccessDeniedError, message unless json['status']

      Action::Gitaly.create_from_json(actor, json)
    when HTTP_UNAUTHORIZED, HTTP_NOT_FOUND
      raise AccessDeniedError, message
    else
      raise UnknownError, "#{API_INACCESSIBLE_ERROR}: #{message}"
    end
  rescue JSON::ParserError
    raise UnknownError, API_INACCESSIBLE_ERROR
  end
end
