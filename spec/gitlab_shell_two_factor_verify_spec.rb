require_relative 'spec_helper'

require 'open3'
require 'json'

describe 'bin/gitlab-shell 2fa_verify' do
  include_context 'gitlab shell'

  let(:env) do
    { 'SSH_CONNECTION' => 'fake',
      'SSH_ORIGINAL_COMMAND' => '2fa_verify' }
  end

  let(:correct_otp) { '123456' }

  before(:context) do
    write_config('gitlab_url' => "http+unix://#{CGI.escape(tmp_socket_path)}")
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/two_factor_manual_otp_check') do |req, res|
      res.content_type = 'application/json'
      res.status = 200

      params = JSON.parse(req.body)

      res.body = if params['otp_attempt'] == correct_otp
                   { success: true }.to_json
                 else
                   { success: false, message: 'boom!' }.to_json
                 end
    end

    server.mount_proc('/api/v4/internal/two_factor_push_otp_check') do |req, res|
      res.content_type = 'application/json'
      res.status = 200

      params = JSON.parse(req.body)
      id = params['key_id'] || params['user_id'].to_s

      if id == '100'
        res.body = { success: false, message: 'boom!' }.to_json
      else
        res.body = { success: true }.to_json
      end
    end

    server.mount_proc('/api/v4/internal/discover') do |req, res|
      res.status = 200
      res.content_type = 'application/json'

      if req.query['username'] == 'someone'
        res.body = { id: 100, name: 'Some User', username: 'someuser' }.to_json
      else
        res.body = { id: 101, name: 'Another User', username: 'another' }.to_json
      end
    end
  end

  describe 'entering OTP manually' do
    let(:cmd) { "#{gitlab_shell_path} key-100" }

    context 'when key is provided' do
      it 'asks a user for a correct OTP' do
        verify_successful_otp_verification!(cmd)
      end
    end

    context 'when username is provided' do
      let(:cmd) { "#{gitlab_shell_path} username-someone" }

      it 'asks a user for a correct OTP' do
        verify_successful_otp_verification!(cmd)
      end
    end

    it 'shows an error when an invalid otp is provided' do
      Open3.popen2(env, cmd) do |stdin, stdout|
        asks_for_otp(stdout)
        stdin.puts('000000')

        expect(stdout.flush.read).to eq("\nOTP validation failed: boom!\n")
      end
    end
  end

  describe 'authorizing via push' do
    context 'when key is provided' do
      let(:cmd) { "#{gitlab_shell_path} key-101" }

      it 'asks a user for a correct OTP' do
        verify_successful_push_verification!(cmd)
      end
    end

    context 'when username is provided' do
      let(:cmd) { "#{gitlab_shell_path} username-another" }

      it 'asks a user for a correct OTP' do
        verify_successful_push_verification!(cmd)
      end
    end
  end

  def verify_successful_otp_verification!(cmd)
    Open3.popen2(env, cmd) do |stdin, stdout|
      asks_for_otp(stdout)
      stdin.puts(correct_otp)

      expect(stdout.flush.read).to eq("\nOTP validation successful. Git operations are now allowed.\n")
    end
  end

  def verify_successful_push_verification!(cmd)
    Open3.popen2(env, cmd) do |stdin, stdout|
      asks_for_otp(stdout)

      expect(stdout.flush.read).to eq("\nOTP has been validated by Push Authentication. Git operations are now allowed.\n")
    end
  end

  def asks_for_otp(stdout)
    expect(stdout.gets(5)).to eq('OTP: ')
  end
end
