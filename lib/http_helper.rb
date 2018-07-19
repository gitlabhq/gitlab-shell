module HTTPHelper
  READ_TIMEOUT = 300

  protected

  def config
    @config ||= GitlabConfig.new
  end

  def base_api_endpoint
    "#{config.gitlab_url}/api/v4"
  end

  def internal_api_endpoint
    "#{base_api_endpoint}/internal"
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
    $logger.debug('Performing request', method: method.to_s.upcase, url: url)

    uri = URI.parse(url)

    http = http_client_for(uri, options)
    request = http_request_for(method, uri, params)

    begin
      start_time = Time.new
      response = http.start { http.request(request) }
    rescue => e
      $logger.warn('Failed to connect', method: method.to_s.upcase, url: url, error: e)
      raise GitlabNet::ApiUnreachableError
    ensure
      $logger.info('finished HTTP request', method: method.to_s.upcase, url: url, duration: Time.new - start_time)
    end

    if response.code == "200"
      $logger.debug('Received response', code: response.code, body: response.body)
    else
      $logger.error('Call failed', method: method.to_s.upcase, url: url, code: response.code, body: response.body)
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
