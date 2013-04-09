require 'yaml'
require 'net/http'
require 'logger'

$gitlabConfig = Object.new
class << $gitlabConfig
  attr_reader :config, :http_opts, :logger

  def setup
    @config = YAML.load_file(File.join(ROOT_PATH, 'config.yml'))
    @http_opts = Hash.new

    # Setup our logger
    @logger = Logger.new(error_log) if not error_log.nil?   # Probably needs pretty error message
    @logger = Logger.new('/dev/null') if @logger.nil?
    @logger.level = Logger::INFO
    
    @config["base_url"] = URI.parse(base_url.to_s + '/api/v3/internal')
    @config["gitlab_url"] = base_url.to_s

    # If our url scheme is https or we're explicitly told to use https turn it on
    if ((base_url.scheme == 'https') or \
      ((not http_settings['use_ssl'].nil? ) \
        and \
       (http_settings['use_ssl'] == true)) \
      )

      # Update our urls to be correct in memory in case of config conflict
      # Prefer to accidentally use https on an http url than vice versa!
      @config["base_url"].scheme = "https"
      @config["gitlab_url"] = @config["base_url"].to_s
      @http_opts[:use_ssl] = true

      if not http_settings['ssl_version'].nil?
        @http_opts[:ssl_version] = http_settings['ssl_version']
      else
        @http_opts[:ssl_version] = :TLSv1_client
      end

      @logger.debug {"Using TLS! base #{@config["gitlab_url"]} ver: #{@http_opts[:ssl_version].to_s}"}

      if not http_settings['self_signed_cert'].nil? and http_settings['self_signed_cert'] == true
        @http_opts[:verify_mode] = OpenSSL::SSL::VERIFY_NONE
        @logger.error {"NOT VERIFYING X509 HOSTNAMES OR CERTIFICATION PATHS!!!"}
      else
        @http_opts[:verify_mode] = OpenSSL::SSL::VERIFY_PEER|OpenSSL::SSL::VERIFY_FAIL_IF_NO_PEER_CERT
        if not http_settings['verify_depth'].nil? and http_settings['verify_depth'] <= 65535
          @http_opts[:verify_depth] = http_settings['verify_depth']
        else
          @http_opts[:verify_depth] = 2
        end

        if not http_settings['ssl_ca_file'].nil?
          throw Errno::EACCES unless File.open(http_settings['ssl_ca_file'],'r') {|s| true}
          @http_opts[:ca_file] = http_settings['ssl_ca_file']
        elsif not http_settings['ssl_ca_path'].nil?
          throw Errno::EACCES unless Dir.open(http_settings['ssl_ca_path'],'r') {|s| true}
          @http_opts[:ca_path] = http_settings['ssl_ca_path']
        else
          @logger.error("Using system configuration for TLS/X509 ca path validation. Set ssl_ca_path or ssl_ca_file http_settings to silence this message.")
          if File.exists?('/etc/ssl/cert.pem') # FreeBSD ca_root_nss port
            @http_opts[:ca_file] = '/etc/ssl/cert.pem'
          elsif File.exists?('/usr/local/share/certs/ca-root-nss.crt') # FreeBSD ca_root_nss port
            @http_opts[:ca_file] = '/usr/local/share/certs/ca-root-nss.crt'
          elsif File.exists?('/opt/local/share/curl/curl-ca-bundle.crt') # Mac OS X macports
            @http_opts[:ca_file] = '/opt/local/share/curl/curl-ca-bundle.crt'
          elsif File.exists?('/etc/ssl/certs') # Ubuntu
            @http_opts[:ca_path] = '/etc/ssl/certs'
          else
            @logger.error {"Could not find fallback TLS/X509 ca path! Hope your environment is sane!"}
          end
        end
        @logger.info {"Validating X509 Certificates using file #{@http_opts[:ca_file]} ."} if not @http_opts[:ca_file].nil?
        @logger.info {"Validating X509 Certificates using path #{@http_opts[:ca_path]} ."} if not @http_opts[:ca_path].nil?
      end
    end
  end

  def repos_path
    @config['repos_path'] ||= "/home/git/repositories"
  end

  def auth_file
    @config['auth_file'] ||= "/home/git/.ssh/authorized_keys"
  end

  def gitlab_url
    @config['gitlab_url'] ||= "http://localhost/"
  end

  def base_url
    @config['base_url'] ||= URI.parse(gitlab_url)
  end

  def http_settings
    @config['http_settings'] ||= {}
  end

  def access_log
    @config['access_log'] ||= "/home/git/gitlab/log/gitlab-shell_access.log"
  end

  def error_log
    @config['error_log'] ||= "/home/git/gitlab/log/gitlab-shell_error.log"
  end
end
$gitlabConfig.setup
