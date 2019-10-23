require_relative 'spec_helper'

require 'open3'

describe 'bin/gitlab-shell git-lfs-authentication' do
  include_context 'gitlab shell'

  let(:path) { "https://gitlab.com/repo/path" }
  let(:env) { {'SSH_CONNECTION' => 'fake', 'SSH_ORIGINAL_COMMAND' => 'git-lfs-authenticate project/repo download' } }

  before(:context) do
    write_config("gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}")
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/lfs_authenticate') do |req, res|
      res.content_type = 'application/json'

      key_id = req.query['key_id'] || req.query['user_id']

      unless key_id
        body = JSON.parse(req.body)
        key_id = body['key_id'] || body['user_id'].to_s
      end

      if key_id == '100'
        res.status = 200
        res.body = %{{"username":"john","lfs_token":"sometoken","repository_http_path":"#{path}","expires_in":1800}}
      else
        res.status = 403
      end
    end

    server.mount_proc('/api/v4/internal/allowed') do |req, res|
      res.content_type = 'application/json'

      key_id = req.query['key_id'] || req.query['username']

      unless key_id
        body = JSON.parse(req.body)
        key_id = body['key_id'] || body['username'].to_s
      end

      case key_id
      when '100', 'someone' then
        res.status = 200
        res.body = '{"gl_id":"user-100", "status":true}'
      when '101' then
        res.status = 200
        res.body = '{"gl_id":"user-101", "status":true}'
      else
        res.status = 403
      end
    end
  end

  describe 'lfs authentication command' do
    def successful_response
      {
        "header" => {
          "Authorization" => "Basic am9objpzb21ldG9rZW4="
        },
        "href" => "#{path}/info/lfs",
        "expires_in" => 1800
      }.to_json + "\n"
    end

    context 'when the command is allowed' do
      context 'when key is provided' do
        let(:cmd) { "#{gitlab_shell_path} key-100" }

        it 'lfs is successfully authenticated' do
          output, stderr, status = Open3.capture3(env, cmd)

          expect(output).to eq(successful_response)
          expect(status).to be_success
        end
      end

      context 'when username is provided' do
        let(:cmd) { "#{gitlab_shell_path} username-someone" }

        it 'lfs is successfully authenticated' do
          output, stderr, status = Open3.capture3(env, cmd)

          expect(output).to eq(successful_response)
          expect(status).to be_success
        end
      end
    end

    context 'when a user is not allowed to perform an action' do
      let(:cmd) { "#{gitlab_shell_path} key-102" }

      it 'lfs is not authenticated' do
        _, stderr, status = Open3.capture3(env, cmd)

        expect(stderr).not_to be_empty
        expect(status).not_to be_success
      end
    end

    context 'when lfs authentication is forbidden for a user' do
      let(:cmd) { "#{gitlab_shell_path} key-101" }

      it 'lfs is not authenticated' do
        output, stderr, status = Open3.capture3(env, cmd)

        expect(stderr).to be_empty
        expect(output).to be_empty
        expect(status).to be_success
      end
    end

    context 'when an action for lfs authentication is unknown' do
      let(:cmd) { "#{gitlab_shell_path} key-100" }
      let(:env) { {'SSH_CONNECTION' => 'fake', 'SSH_ORIGINAL_COMMAND' => 'git-lfs-authenticate project/repo unknown' } }

      it 'the command is disallowed' do
        divider = "remote: \nremote: ========================================================================\nremote: \n"
        _, stderr, status = Open3.capture3(env, cmd)

        expect(stderr).to eq("#{divider}remote: Disallowed command\n#{divider}")
        expect(status).not_to be_success
      end
    end
  end
end
