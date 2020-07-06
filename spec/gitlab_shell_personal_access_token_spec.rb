require_relative 'spec_helper'

require 'json'
require 'open3'

describe 'bin/gitlab-shell personal_access_token' do
  include_context 'gitlab shell'

  before(:context) do
    write_config("gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}")
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/personal_access_token') do |req, res|
      params = JSON.parse(req.body)

      res.content_type = 'application/json'
      res.status = 200

      if params['key_id'] == '000'
        res.body = { success: false, message: "Something wrong!"}.to_json
      else
        res.body = {
          success: true,
          token: 'aAY1G3YPeemECgUvxuXY',
          scopes: params['scopes'],
          expires_at: (params['expires_at'] && '9001-12-01')
        }.to_json
      end
    end

    server.mount_proc('/api/v4/internal/discover') do |req, res|
      res.status = 200
      res.content_type = 'application/json'
      res.body = '{"id":100, "name": "Some User", "username": "someuser"}'
    end
  end

  describe 'command' do
    let(:key_id) { 'key-100' }

    let(:output) do
      env = {
        'SSH_CONNECTION'       => 'fake',
        'SSH_ORIGINAL_COMMAND' => "personal_access_token #{args}"
      }
      Open3.popen2e(env, "#{gitlab_shell_path} #{key_id}")[1].read()
    end

    let(:help_message) do
      <<~OUTPUT
        remote: 
        remote: ========================================================================
        remote: 
        remote: Usage: personal_access_token <name> <scope1[,scope2,...]> [ttl_days]
        remote: 
        remote: ========================================================================
        remote: 
      OUTPUT
    end

    context 'without any arguments' do
      let(:args) { '' }

      it 'prints the help message' do
        expect(output).to eq(help_message)
      end
    end

    context 'with only the name argument' do
      let(:args) { 'newtoken' }

      it 'prints the help message' do
        expect(output).to eq(help_message)
      end
    end

    context 'without a ttl argument' do
      let(:args) { 'newtoken api' }

      it 'prints a token without an expiration date' do
        expect(output).to eq(<<~OUTPUT)
          Token:   aAY1G3YPeemECgUvxuXY
          Scopes:  api
          Expires: never
        OUTPUT
      end
    end

    context 'with a ttl argument' do
      let(:args) { 'newtoken read_api,read_user 30' }

      it 'prints a token with an expiration date' do
        expect(output).to eq(<<~OUTPUT)
          Token:   aAY1G3YPeemECgUvxuXY
          Scopes:  read_api,read_user
          Expires: 9001-12-01
        OUTPUT
      end
    end

    context 'with an API error response' do
      let(:args) { 'newtoken api' }
      let(:key_id) { 'key-000' }

      it 'prints the error response' do
        expect(output).to eq(<<~OUTPUT)
          remote: 
          remote: ========================================================================
          remote: 
          remote: Something wrong!
          remote: 
          remote: ========================================================================
          remote: 
        OUTPUT
      end
    end
  end
end
