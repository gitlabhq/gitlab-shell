require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'
require_relative 'httpunix'

class GitlabNet
  class ApiUnreachableError < StandardError; end

  CHECK_TIMEOUT = 5
  READ_TIMEOUT = 300

  def check_access(cmd, repo, actor, changes)
    project_name = repo.gsub("'", "")
    project_name = project_name.gsub(/\.git\Z/, "")
    project_name = project_name.gsub(/\A\//, "")
    changes = changes.join("\n") unless changes.kind_of?(String)

    params = {
      action: cmd,
      changes: changes,
      project: project_name,
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
      GitAccessStatus.new(false, 'API is not accessible')
    end
  end

  def discover(key)
    key_id = key.gsub("key-", "")
    resp = get("#{host}/discover?key_id=#{key_id}")
    JSON.parse(resp.body) rescue nil
  end

  def broadcast_message
    resp = get("#{host}/broadcast_message")
    JSON.parse(resp.body) rescue {}
  end

  def check
    get("#{host}/check", read_timeout: CHECK_TIMEOUT)
  end

  def authorized_key(fingerprint)
    resp = get("#{host}/authorized_keys?fingerprint=#{fingerprint}")
    JSON.parse(resp.body) if resp.code == "200"
  rescue
    nil
  end

  protected

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
    request_klass = method == :get ? Net::HTTP::Get : Net::HTTP::Post
    request = request_klass.new(uri.request_uri)

    user = config.http_settings['user']
    password = config.http_settings['password']
    request.basic_auth(user, password) if user && password

    request.set_form_data(params.merge(secret_token: secret_token))

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
