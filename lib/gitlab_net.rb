require 'json'

require_relative 'gitlab_access'
require_relative 'gitlab_redis'
require_relative 'gitlab_lfs_authentication'
require_relative 'http_client'

class GitlabNet < HttpClient
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

  def http_request_for(method, uri, params = {})
    super(method, uri, params.merge(secret_token: secret_token))
  end

  def host
    "#{config.gitlab_url}/api/v3/internal"
  end

  def secret_token
    @secret_token ||= File.read config.secret_file
  end
end
