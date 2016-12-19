require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'
require_relative 'gitlab_redis'
require_relative 'gitlab_lfs_authentication'
require_relative 'httpunix'

class GitlabNet
  class ApiUnreachableError < StandardError; end

  CHECK_TIMEOUT = 5
  READ_TIMEOUT = 300

  def check_access(cmd, repo, actor, changes, protocol)
    changes = changes.join("\n") unless changes.kind_of?(String)

    params = {
      action: cmd,
      changes: changes,
      project: project_name(repo),
      protocol: protocol
    }

    if actor =~ /\Akey\-\d+\Z/
      params.merge!(key_id: actor.gsub("key-", ""))
    elsif actor =~ /\Auser\-\d+\Z/
      params.merge!(user_id: actor.gsub("user-", ""))
    end

    url = "#{host}/allowed"
    resp = post(url, params)

    if resp.code == '200'
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
      project: project_name(repo),
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

  def merge_request_urls(repo_name, changes)
    changes = changes.join("\n") unless changes.kind_of?(String)
    changes = changes.encode('UTF-8', 'ASCII', invalid: :replace, replace: '')
    resp = get("#{host}/merge_request_urls?project=#{URI.escape(repo_name)}&changes=#{URI.escape(changes)}")
    JSON.parse(resp.body) rescue []
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

  def project_name(repo)
    project_name = repo.gsub("'", "")
    project_name = project_name.gsub(/\.git\Z/, "")
    project_name.gsub(/\A\//, "")
  end

  def config
    @config ||= GitlabConfig.new
  end

  def host
    "#{config.gitlab_url}/api/v3/internal"
  end

  def http_client_for(uri, options={})
    if uri.is_a?(URI::HTTPUNIX)
      http = Net::HTTPUNIX.new(uri.hostname)
    else
      http = Net::HTTP.new(uri.host, uri.port)
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
    url = uri.request_uri

    case method
    when :get
      url += (url.include?('?') ? '&' : '?') +
             URI.encode_www_form(secret_token: secret_token)
      request = Net::HTTP::Get.new(url)
    else
      request = Net::HTTP::Post.new(url)
      request.set_form_data(params.merge(secret_token: secret_token))
    end

    user = config.http_settings['user']
    password = config.http_settings['password']
    request.basic_auth(user, password) if user && password

    request
  end

  def request(method, url, params = {}, options={})
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
        sprintf('%s %s %0.5f', method.to_s.upcase, url, Time.new - start_time)
      end
    end

    if response.code == "200"
      $logger.debug "Received response #{response.code} => <#{response.body}>."
    else
      $logger.error "API call <#{method.to_s.upcase} #{url}> failed: #{response.code} => <#{response.body}>."
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
