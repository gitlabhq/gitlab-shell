require_relative 'spec_helper'

require 'open3'

describe 'bin/gitlab-shell 2fa_recovery_codes' do
  include_context 'gitlab shell'

  let(:env) { {'SSH_CONNECTION' => 'fake', 'SSH_ORIGINAL_COMMAND' => '2fa_recovery_codes' } }

  before(:context) do
    write_config("gitlab_url" => "http+unix://#{CGI.escape(tmp_socket_path)}")
  end

  def mock_server(server)
    server.mount_proc('/api/v4/internal/two_factor_recovery_codes') do |req, res|
      res.content_type = 'application/json'
      res.status = 200

      key_id = req.query['key_id'] || req.query['user_id']

      unless key_id
        body = JSON.parse(req.body)
        key_id = body['key_id'] || body['user_id'].to_s
      end

      if key_id == '100'
        res.body = '{"success":true, "recovery_codes": ["1", "2"]}'
      else
        res.body = '{"success":false, "message": "Forbidden!"}'
      end
    end

    server.mount_proc('/api/v4/internal/discover') do |req, res|
      res.status = 200
      res.content_type = 'application/json'
      res.body = '{"id":100, "name": "Some User", "username": "someuser"}'
    end
  end

  describe 'dialog for regenerating recovery keys' do
    context 'when the user agrees to regenerate keys' do
      def verify_successful_regeneration!(cmd)
        Open3.popen2(env, cmd) do |stdin, stdout|
          expect(stdout.gets).to eq("Are you sure you want to generate new two-factor recovery codes?\n")
          expect(stdout.gets).to eq("Any existing recovery codes you saved will be invalidated. (yes/no)\n")

          stdin.puts('yes')

          expect(stdout.flush.read).to eq(
            "\nYour two-factor authentication recovery codes are:\n\n" \
            "1\n2\n\n" \
            "During sign in, use one of the codes above when prompted for\n" \
            "your two-factor code. Then, visit your Profile Settings and add\n" \
            "a new device so you do not lose access to your account again.\n"
          )
        end
      end

      context 'when key is provided' do
        let(:cmd) { "#{gitlab_shell_path} key-100" }

        it 'the recovery keys are regenerated' do
          verify_successful_regeneration!(cmd)
        end
      end

      context 'when username is provided' do
        let(:cmd) { "#{gitlab_shell_path} username-someone" }

        it 'the recovery keys are regenerated' do
          verify_successful_regeneration!(cmd)
        end
      end
    end

    context 'when the user disagrees to regenerate keys' do
      let(:cmd) { "#{gitlab_shell_path} key-100" }

      it 'the recovery keys are not regenerated' do
        Open3.popen2(env, cmd) do |stdin, stdout|
          expect(stdout.gets).to eq("Are you sure you want to generate new two-factor recovery codes?\n")
          expect(stdout.gets).to eq("Any existing recovery codes you saved will be invalidated. (yes/no)\n")

          stdin.puts('no')

          expect(stdout.flush.read).to eq(
            "\nNew recovery codes have *not* been generated. Existing codes will remain valid.\n"
          )
        end
      end
    end

    context 'when API error occurs' do
      let(:cmd) { "#{gitlab_shell_path} key-101" }

      context 'when the user agrees to regenerate keys' do
        it 'the recovery keys are regenerated' do
          Open3.popen2(env, cmd) do |stdin, stdout|
            expect(stdout.gets).to eq("Are you sure you want to generate new two-factor recovery codes?\n")
            expect(stdout.gets).to eq("Any existing recovery codes you saved will be invalidated. (yes/no)\n")

            stdin.puts('yes')

            expect(stdout.flush.read).to eq("\nAn error occurred while trying to generate new recovery codes.\nForbidden!\n")
          end
        end
      end
    end
  end
end
