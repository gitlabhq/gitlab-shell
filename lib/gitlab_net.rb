require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'
require_relative 'gitlab_logger'
require_relative 'gitlab_access'

class GitlabNet
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

  def check
    get("#{host}/check")
  end

  protected

  def config
    @config ||= GitlabConfig.new
  end

  def host
    "#{config.gitlab_url}/api/v3/internal"
  end

  def http_client_for(url)
    Net::HTTP.new(url.host, url.port).tap do |http|
      if URI::HTTPS === url
        http.use_ssl = true
        http.cert_store = cert_store
        http.verify_mode = OpenSSL::SSL::VERIFY_NONE if config.http_settings['self_signed_cert']
      end
    end
  end

  def http_request_for(url, method = :get)
    user = config.http_settings['user']
    password = config.http_settings['password']

    if method == :get
      Net::HTTP::Get.new(url.request_uri).tap { |r| r.basic_auth(user, password) if user && password }
    else
      Net::HTTP::Post.new(url.request_uri).tap { |r| r.basic_auth(user, password) if user && password }
    end
  end

  def get(url)
    $logger.debug "Performing GET #{url}"

    url = URI.parse(url)
    request = http_request_for url
    request.set_form_data(secret_token: secret_token)

    resp = nil
    if config.unix_socket then
      sock = Net::BufferedIO.new(UNIXSocket.new(config.unix_socket))
      request.exec(sock, "1.1", url.request_uri)

      resp = Net::HTTPResponse.read_new(sock)
      resp.reading_body(sock, true) { }
    else
      http = http_client_for url
      http.start
      resp = http.request(request)
    end

    if resp.code == "200"
      $logger.debug { "Received response #{resp.code} => <#{resp.body}>." }
    else
      $logger.error { "API call <GET #{url}> failed: #{resp.code} => <#{resp.body}>." }
    end
    resp
  end

  def post(url, params)
    $logger.debug "Performing POST #{url}"

    url = URI.parse(url)
    http = http_client_for(url)
    request = http_request_for(url, :post)
    request.set_form_data(params.merge(secret_token: secret_token))

    http.start { |http| http.request(request) }.tap do |resp|
      if resp.code == "200"
        $logger.debug { "Received response #{resp.code} => <#{resp.body}>." }
      else
        $logger.error { "API call <POST #{url}> failed: #{resp.code} => <#{resp.body}>." }
      end
    end
  end

  def cert_store
    @cert_store ||= OpenSSL::X509::Store.new.tap do |store|
      store.set_default_paths

      if ca_file = config.http_settings['ca_file']
        store.add_file(ca_file)
      end

      if ca_path = config.http_settings['ca_path']
        store.add_path(ca_path)
      end
    end
  end

  def secret_token
    @secret_token ||= File.read File.join(ROOT_PATH, '.gitlab_shell_secret')
  end
end
