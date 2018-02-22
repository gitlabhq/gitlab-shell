require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'
require_relative 'gitlab_lfs_authentication'
require_relative 'httpunix'

class GitlabNet # rubocop:disable Metrics/ClassLength
  class ApiUnreachableError < StandardError; end
  class NotFound < StandardError; end

  CHECK_TIMEOUT = 5
  READ_TIMEOUT = 300

  def check_access(cmd, gl_repository, repo, actor, changes, protocol, env: {})
    changes = changes.join("\n") unless changes.is_a?(String)

    params = {
      action: cmd,
      changes: changes,
      gl_repository: gl_repository,
      project: sanitize_path(repo),
      protocol: protocol,
      env: env
    }

    if actor =~ /\Akey\-\d+\Z/
      params[:key_id] = actor.gsub("key-", "")
    elsif actor =~ /\Auser\-\d+\Z/
      params[:user_id] = actor.gsub("user-", "")
    end

    url = "#{host}/allowed"
    resp = post(url, params)

    if resp.code == '200'
      GitAccessStatus.create_from_json(resp.body)
    else
      GitAccessStatus.new(false,
                          'API is not accessible',
                          gl_repository: nil,
                          gl_username: nil,
                          repository_path: nil,
                          gitaly: nil)
    end
  end

  def discover(key)
    key_id = key.gsub("key-", "")
    resp = get("#{host}/discover?key_id=#{key_id}")
    JSON.parse(resp.body) rescue nil
  end

  def lfs_authenticate(key, repo)
    params = {
      project: sanitize_path(repo),
      key_id: key.gsub('key-', '')
    }

    resp = post("#{host}/lfs_authenticate", params)

    if resp.code == '200'
      GitlabLfsAuthentication.build_from_json(resp.body)
    end
  end

  def broadcast_message
    resp = get("#{host}/broadcast_message")
    JSON.parse(resp.body) rescue {}
  end

  def merge_request_urls(gl_repository, repo_path, changes)
    changes = changes.join("\n") unless changes.is_a?(String)
    changes = changes.encode('UTF-8', 'ASCII', invalid: :replace, replace: '')
    url = "#{host}/merge_request_urls?project=#{URI.escape(repo_path)}&changes=#{URI.escape(changes)}"
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
    get("#{host}/check", read_timeout: CHECK_TIMEOUT)
  end

  def authorized_key(key)
    resp = get("#{host}/authorized_keys?key=#{URI.escape(key, '+/=')}")
    JSON.parse(resp.body) if resp.code == "200"
  rescue
    nil
  end

  def two_factor_recovery_codes(key)
    key_id = key.gsub('key-', '')
    resp = post("#{host}/two_factor_recovery_codes", key_id: key_id)

    JSON.parse(resp.body) if resp.code == '200'
  rescue
    {}
  end

  def notify_post_receive(gl_repository, repo_path)
    params = { gl_repository: gl_repository, project: repo_path }
    resp = post("#{host}/notify_post_receive", params)

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
    resp = post("#{host}/post_receive", params)

    raise NotFound if resp.code == '404'

    JSON.parse(resp.body) if resp.code == '200'
  end

  def pre_receive(gl_repository)
    resp = post("#{host}/pre_receive", gl_repository: gl_repository)

    raise NotFound if resp.code == '404'

    JSON.parse(resp.body) if resp.code == '200'
  end

  protected

  def sanitize_path(repo)
    repo.delete("'")
  end

  def config
    @config ||= GitlabConfig.new
  end

  def host
    "#{config.gitlab_url}/api/v4/internal"
  end

  def http_client_for(uri, options = {})
    http = if uri.is_a?(URI::HTTPUNIX)
             Net::HTTPUNIX.new(uri.hostname)
           else
             Net::HTTP.new(uri.host, uri.port)
           end

    http.read_timeout = options[:read_timeout] || read_timeout

    if uri.is_a?(URI::HTTPS)
      http.use_ssl = true
      http.cert_store = cert_store
      http.verify_mode = OpenSSL::SSL::VERIFY_NONE if config.http_settings['self_signed_cert']
    end

    http
  end

  def http_request_for(method, uri, params = {})
    request_klass = method == :get ? Net::HTTP::Get : Net::HTTP::Post
    request = request_klass.new(uri.request_uri)

    user = config.http_settings['user']
    password = config.http_settings['password']
    request.basic_auth(user, password) if user && password

    request.set_form_data(params.merge(secret_token: secret_token))

    if uri.is_a?(URI::HTTPUNIX)
      # The HTTPUNIX HTTP client does not set a correct Host header. This can
      # lead to 400 Bad Request responses.
      request['Host'] = 'localhost'
    end

    request
  end

  def request(method, url, params = {}, options = {})
    $logger.debug "Performing #{method.to_s.upcase} #{url}"

    uri = URI.parse(url)

    http = http_client_for(uri, options)
    request = http_request_for(method, uri, params)

    begin
      start_time = Time.new
      response = http.start { http.request(request) }
    rescue => e
      $logger.warn "Failed to connect to internal API <#{method.to_s.upcase} #{url}>: #{e.inspect}"
      raise ApiUnreachableError
    ensure
      $logger.info do
        sprintf('%s %s %0.5f', method.to_s.upcase, url, Time.new - start_time) # rubocop:disable Style/FormatString
      end
    end

    if response.code == "200"
      $logger.debug "Received response #{response.code} => <#{response.body}>."
    else
      $logger.error "API call <#{method.to_s.upcase} #{url}> failed: #{response.code} => <#{response.body}>."
    end

    response
  end

  def get(url, options = {})
    request(:get, url, {}, options)
  end

  def post(url, params)
    request(:post, url, params)
  end

  def cert_store
    @cert_store ||= begin
      store = OpenSSL::X509::Store.new
      store.set_default_paths

      ca_file = config.http_settings['ca_file']
      store.add_file(ca_file) if ca_file

      ca_path = config.http_settings['ca_path']
      store.add_path(ca_path) if ca_path

      store
    end
  end

  def secret_token
    @secret_token ||= File.read config.secret_file
  end

  def read_timeout
    config.http_settings['read_timeout'] || READ_TIMEOUT
  end
end
