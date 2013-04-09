require 'net/http'
require 'openssl'
require 'json'

require_relative 'gitlab_config'

class GitlabNet
  def allowed?(cmd, repo, key, ref)
    project_name = repo.gsub("'", "")
    project_name = project_name.gsub(/\.git\Z/, "")
    project_name = project_name.gsub(/\A\//, "")

    key_id = key.gsub("key-", "")

    url = "#{host}/allowed?key_id=#{key_id}&action=#{cmd}&ref=#{ref}&project=#{project_name}"

    config.logger.debug {"Trying #{url} ."}
    resp = get(url)

    !!(resp.code == '200' && resp.body == 'true')
  end

  def discover(key)
    key_id = key.gsub("key-", "")
    config.logger.debug {"Trying #{host}/discover?key_id=#{key_id} ."}
    resp = get("#{host}/discover?key_id=#{key_id}")
    JSON.parse(resp.body) rescue nil
  end

  def check
    config.logger.debug {"Trying #{host}/check ."}
    get("#{host}/check")
  end

  protected

  def base_uri
    config.base_url
  end

  def config
    @config ||= $gitlabConfig
  end

  def host
    config.gitlab_url
  end

  def http
    if @http.nil?
      @http = Net::HTTP.new(base_uri.host, base_uri.port)
      unless config.http_opts[:use_ssl].nil? or config.http_opts[:use_ssl] == false
        http.use_ssl = config.http_opts[:use_ssl]
        http.verify_mode = config.http_opts[:verify_mode]
        http.verify_depth = config.http_opts[:verify_depth]
        http.ca_file = config.http_opts[:ca_file]
        http.ca_path = config.http_opts[:ca_path]
        # This is a first attempt at sane ssl cert verification.
        # subjectAltName and CN checking
        http.verify_callback = lambda do |preverify_ok, ssl_context|
          # Newer Ruby uses keyed siphash for it's hashes so this is pretty safe to use
          # as a known cert lookup table in this fashion. If ruby in use doesn't it should
          # be upgarded as the previous hash used is easily collidable and this could be
          # tricked with malformed subjectAltNames/CNs (though we only connect to one host ...)
          if @known_certs.nil?
            @known_certs = Hash.new
          end
          success = false
          have_altname = false
          cert = ssl_context.chain[0] if not ssl_context.chain[0].nil?
          return false if cert.nil?

          # Compares two host names using browser/x509 wildcard matching rules
          if @check_cert_name.nil?
            @check_cert_name = lambda do |x,y|
              success = false
              fqdna = x.split('.')
              hosta = y.split('.')
              if fqdna[0] =~ /[*]/
                fqdnf = fqdna[0].sub(/[*]/,'[^.]*')
                hostf = hosta[0]
                fqdna.delete_at[0]
                hosta.delete_at[0]
                if fqdna.join('.') == hosta.join('.')
                  success = true if hostf =~ Regexp.new(fqdnf)
                end
              else
                success = true if x == y
              end
              return success
            end
          end

          # Cache verification based on keyed hash of the der form of the cert
          if @remember_cert.nil?
            @remember_cert = lambda do |match,host,cert|
              if @known_certs[cert.to_der.hash].nil?
                @known_certs[cert.to_der.hash] = Hash.new
              end
              @known_certs[cert.to_der.hash].store(host.hash, match)
            end
          end

          if  (not @known_certs[cert.to_der.hash].nil?) \
            and \
              (not @known_certs[cert.to_der.hash].fetch(base_uri.host.hash).nil?) \
            and \
              @check_cert_name.call(@known_certs[cert.to_der.hash].fetch(base_uri.host.hash),base_uri.host)
            success = (preverify_ok && true)
          elsif preverify_ok
            # First we look for subjectAltNames that match our host! (rfc2818)
            cert.extensions.each do |ext|
              if (ext.oid == 'subjectAltName')
                have_altname = true
                ext.value.split(', ').each do |v|
                  v.sub!(/^(DNS|IP):/,'')
                  success = true if @check_cert_name.call(v,base_uri.host)
                  if success
                    @remember_cert.call(v,base_uri.host,cert)
                    break 2
                  end
                end
              end
            end
            # Fallback to matching the CN - We only match against the longest CN value in the subject.
            # This should be correct per rfc2818 as the longest will be the most specific.
            if not have_altname
              certcn = (cert.subject.to_s.match(/\/CN=([^\/]+)\//).captures)[0]
              success = true if @check_cert_name.call(certcn,base_uri.host)
              @remember_cert.call(certcn,base_uri.host,cert) if success
            end
            # Verification fails!
          end
          return success
        end
      end
    end
    @http
  end

  def get(url)
    url = URI.parse(url)

    request = Net::HTTP::Get.new(url.request_uri)
    if config.http_settings['user'] && config.http_settings['password']
      request.basic_auth config.http_settings['user'], config.http_settings['password']
    end

    http.start {|http| http.request(request) }
  end
end
