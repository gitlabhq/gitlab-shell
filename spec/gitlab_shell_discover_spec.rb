require_relative 'spec_helper'

require 'open3'

describe 'bin/gitlab-shell' do
  include_context 'gitlab shell'

  before(:context) do
    write_config("gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}")
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/discover') do |req, res|
      identifier = req.query['key_id'] || req.query['username'] || req.query['user_id']
      known_identifiers = %w(10 someuser 100)
      if known_identifiers.include?(identifier)
        res.status = 200
        res.content_type = 'application/json'
        res.body = '{"id":1, "name": "Some User", "username": "someuser"}'
      elsif identifier == 'broken_message'
        res.status = 401
        res.body = '{"message": "Forbidden!"}'
      elsif identifier && identifier != 'broken'
        res.status = 200
        res.content_type = 'application/json'
        res.body = 'null'
      else
        res.status = 500
      end
    end
  end

  def run!(args, env: {'SSH_CONNECTION' => 'fake'})
    cmd = [
      gitlab_shell_path,
      args
    ].flatten.compact.join(' ')

    Open3.capture3(env, cmd)
  end

  describe 'results with keys' do
    # Basic valid input
    it 'succeeds and prints username when a valid known key id is given' do
      output, _, status = run!(["key-100"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints username when a valid known username is given' do
      output, _, status = run!(["username-someuser"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    # Valid but unknown input
    it 'succeeds and prints Anonymous when a valid unknown key id is given' do
      output, _, status = run!(["key-12345"])

      expect(output).to eq("Welcome to GitLab, Anonymous!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints Anonymous when a valid unknown username is given' do
      output, _, status = run!(["username-unknown"])

      expect(output).to eq("Welcome to GitLab, Anonymous!\n")
      expect(status).to be_success
    end

    it 'gets an ArgumentError on invalid input (empty)' do
      _, stderr, status = run!([])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    it 'gets an ArgumentError on invalid input (unknown)' do
      _, stderr, status = run!(["whatever"])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    it 'gets an ArgumentError on invalid input (multiple unknown)' do
      _, stderr, status = run!(["this", "is", "all", "invalid"])

      expect(stderr).to match(/who='' is invalid/)
      expect(status).not_to be_success
    end

    # Not so basic valid input
    # (https://gitlab.com/gitlab-org/gitlab-shell/issues/145)
    it 'succeeds and prints username when a valid known key id is given in the middle of other input' do
      output, _, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "key-100", "2foo"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    it 'succeeds and prints username when a valid known username is given in the middle of other input' do
      output, _, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-someuser" ,"foo"])

      expect(output).to eq("Welcome to GitLab, @someuser!\n")
      expect(status).to be_success
    end

    it 'outputs "Only SSH allowed"' do
      _, stderr, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-someuser"], env: {'SSH_CONNECTION' => ''})

      expect(stderr).to eq("Only SSH allowed\n")
      expect(status).not_to be_success
    end

    it 'returns an error message when the API call fails with a message' do
      _, stderr, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-broken_message"])

      expect(stderr).to match(/Failed to get username: Forbidden!/)
      expect(status).not_to be_success
    end

    it 'returns an error message when the API call fails without a message' do
      _, stderr, status = run!(["-c/usr/share/webapps/gitlab-shell/bin/gitlab-shell", "username-broken"])

      expect(stderr).to match(/Failed to get username: Internal API unreachable/)
      expect(status).not_to be_success
    end
  end
end
