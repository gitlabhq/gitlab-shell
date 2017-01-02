require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'
require_relative 'gitlab_redis'
require_relative 'gitlab_lfs_authentication'
require_relative 'gitlab_excon'


class GitlabNet
  class ApiUnreachableError < StandardError; end

  CHECK_TIMEOUT = 5
  READ_TIMEOUT = 300

  def check_access(cmd, repo, actor, changes, protocol, env: {})
    changes = changes.join("\n") unless changes.kind_of?(String)

    params = {
      action: cmd,
      changes: changes,
      project: sanitize_path(repo),
      protocol: protocol,
      env: env
    }

    if actor =~ /\Akey\-\d+\Z/
      params.merge!(key_id: actor.gsub("key-", ""))
    elsif actor =~ /\Auser\-\d+\Z/
      params.merge!(user_id: actor.gsub("user-", ""))
    end

    url = "#{host}/allowed"
    resp = post(url, params)

    if resp.status == 200
      GitAccessStatus.create_from_json(resp.body)
    else
      GitAccessStatus.new(false, 'API is not accessible', nil)
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

    if resp.status == 200
      GitlabLfsAuthentication.build_from_json(resp.body)
    end
  end

  def broadcast_message
    resp = get("#{host}/broadcast_message")
    JSON.parse(resp.body) rescue {}
  end

  def merge_request_urls(repo_path, changes)
    changes = changes.join("\n") unless changes.kind_of?(String)
    changes = changes.encode('UTF-8', 'ASCII', invalid: :replace, replace: '')
    resp = get("#{host}/merge_request_urls?project=#{URI.escape(repo_path)}&changes=#{URI.escape(changes)}")
    JSON.parse(resp.body) rescue []
  end

  def check
    get("#{host}/check", read_timeout: CHECK_TIMEOUT)
  end

  def authorized_key(key)
    resp = get("#{host}/authorized_keys?key=#{URI.escape(key, '+/=')}")
    JSON.parse(resp.body) if resp.code == 200
  rescue
    nil
  end

  def two_factor_recovery_codes(key)
    key_id = key.gsub('key-', '')
    resp = post("#{host}/two_factor_recovery_codes", key_id: key_id)

    JSON.parse(resp.body) if resp.status == 200
  rescue
    {}
  end

  def redis_client
    redis_config = config.redis
    database = redis_config['database'] || 0
    params = {
      host: redis_config['host'] || '127.0.0.1',
      port: redis_config['port'] || 6379,
      db: database
    }

    if redis_config.has_key?('sentinels')
      params[:sentinels] = redis_config['sentinels']
                           .select { |s| s['host'] && s['port'] }
                           .map { |s| { host: s['host'], port: s['port'] } }
    end

    if redis_config.has_key?("socket")
      params = { path: redis_config['socket'], db: database }
    elsif redis_config.has_key?("pass")
      params[:password] = redis_config['pass']
    end

    Redis.new(params)
  end

  protected

  def sanitize_path(repo)
    repo.gsub("'", "")
  end

  def config
    @config ||= GitlabConfig.new
  end

  def host
    "#{config.gitlab_url}/api/v3/internal"
  end

  def http_client_for(url)
    excon_options = {}

    user = config.http_settings['user']
    password = config.http_settings['password']
    if user && password
      excon_options[:user] = user
      excon_options[:password] = password
    end

    uri = URI.parse(url)
    if uri.scheme == 'http+unix'
      excon_options[:socket] = URI.unescape(uri.host)
      url.sub!('http+', '')
      url.sub!(uri.host, '/')
    else
      excon_options[:ssl_verify_peer] = !config.http_settings['self_signed_cert']
      excon_options[:ssl_cert_store] = cert_store
    end
    Excon.new(url, excon_options)
  end

  def request(method, url, params = {}, options={})
    $logger.debug "Performing #{method.to_s.upcase} #{url}"

    http = http_client_for(url)
    request_options = {
      method: method,
      read_timeout: options[:read_timeout] || read_timeout,
      body: URI.encode_www_form(params.merge(secret_token: secret_token)),
      headers: { "Content-Type" => "application/x-www-form-urlencoded" }
    }

    begin
      start_time = Time.new
      response = http.request(request_options)
    rescue => e
      $logger.warn "Failed to connect to internal API <#{method.to_s.upcase} #{url}>: #{e.inspect}"
      raise ApiUnreachableError
    ensure
      $logger.info do
        sprintf('%s %s %0.5f', method.to_s.upcase, url, Time.new - start_time)
      end
    end

    if response.status == 200
      $logger.debug "Received response #{response.status} => <#{response.body}>."
    else
      $logger.error "API call <#{method.to_s.upcase} #{url}> failed: #{response.status} => <#{response.body}>."
    end

    response
  end

  def get(url, options={})
    request(:get, url, {}, options)
  end

  def post(url, params)
    request(:post, url, params)
  end

  def cert_store
    @cert_store ||= begin
      store = OpenSSL::X509::Store.new
      store.set_default_paths

      if ca_file = config.http_settings['ca_file']
        store.add_file(ca_file)
      end

      if ca_path = config.http_settings['ca_path']
        store.add_path(ca_path)
      end

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
