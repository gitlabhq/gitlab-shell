module GitlabKeys
  class KeyError < StandardError; end

  def self.command(whatever)
    "#{ROOT_PATH}/bin/gitlab-shell #{whatever}"
  end

  def self.command_key(key_id)
    unless /\A[a-z0-9-]+\z/ =~ key_id
      raise KeyError, "Invalid key_id: #{key_id.inspect}"
    end

    command(key_id)
  end

  def self.whatever_line(command, trailer)
    "command=\"#{command}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty #{trailer}"
  end

  def self.key_line(key_id, public_key)
    public_key.chomp!

    if public_key.include?("\n")
      raise KeyError, "Invalid public_key: #{public_key.inspect}"
    end

    whatever_line(command_key(key_id), public_key)
  end

  def self.principal_line(username_key_id, principal)
    principal.chomp!

    if principal.include?("\n")
      raise KeyError, "Invalid principal: #{principal.inspect}"
    end

    whatever_line(command_key(username_key_id), principal)
  end
end
