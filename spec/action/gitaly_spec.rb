require_relative '../spec_helper'
require_relative '../../lib/action/gitaly'

describe Action::Gitaly do
  let(:git_trace_log_file_valid) { '/tmp/git_trace_performance.log' }
  let(:git_trace_log_file_invalid) { "/bleep-bop#{git_trace_log_file_valid}" }
  let(:git_trace_log_file_relative) { "..#{git_trace_log_file_valid}" }
  let(:key_id) { '1' }
  let(:key_str) { 'key-1' }
  let(:key) { Actor::Key.new(key_id) }
  let(:gl_repository) { 'project-1' }
  let(:gl_username) { 'testuser' }
  let(:git_config_options) { ['receive.MaxInputSize=10000'] }
  let(:git_protocol) { 'version=2' }
  let(:tmp_repos_path) { File.join(ROOT_PATH, 'tmp', 'repositories') }
  let(:repo_name) { 'gitlab-ci.git' }
  let(:repository_path) { File.join(tmp_repos_path, repo_name) }
  let(:gitaly_address) { 'unix:gitaly.socket' }
  let(:gitaly_token) { '123456' }
  let(:gitaly) do
    {
      'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default' },
      'address' => gitaly_address,
      'token' => gitaly_token
    }
  end

  describe '.create_from_json' do
    it 'returns an instance of Action::Gitaly' do
      json = {
        "gl_repository" => gl_repository,
        "gl_username" => gl_username,
        "repository_path" => repository_path,
        "gitaly" => gitaly
      }
      expect(described_class.create_from_json(key_id, json)).to be_instance_of(Action::Gitaly)
    end
  end

  subject do
    described_class.new(key, gl_repository, gl_username, git_config_options, git_protocol, repository_path, gitaly)
  end

  describe '#execute' do
    let(:args) { [ repository_path ] }
    let(:base_exec_env) do
      {
        'HOME' => ENV['HOME'],
        'PATH' => ENV['PATH'],
        'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'],
        'LANG' => ENV['LANG'],
        'GL_ID' => key_str,
        'GL_PROTOCOL' => GitlabNet::GL_PROTOCOL,
        'GL_REPOSITORY' => gl_repository,
        'GL_USERNAME' => gl_username,
        'GITALY_TOKEN' => gitaly_token,
      }
    end
    let(:with_trace_exec_env) do
      base_exec_env.merge({
        'GIT_TRACE' => git_trace_log_file,
        'GIT_TRACE_PACKET' => git_trace_log_file,
        'GIT_TRACE_PERFORMANCE' => git_trace_log_file
      })
    end
    let(:gitaly_request) do
      {
        'repository' => gitaly['repository'],
        'gl_repository' => gl_repository,
        'gl_id' => key_str,
        'gl_username' => gl_username,
        'git_config_options' => git_config_options,
        'git_protocol' => git_protocol
      }
    end

    context 'for migrated commands' do
      context 'such as git-upload-pack' do
        let(:git_trace_log_file) { nil }
        let(:command) { 'git-upload-pack' }

        before do
          allow_any_instance_of(GitlabConfig).to receive(:git_trace_log_file).and_return(git_trace_log_file)
        end

        context 'with an invalid config.git_trace_log_file' do
          let(:git_trace_log_file) { git_trace_log_file_invalid }

          it 'returns true' do
            expect(Kernel).to receive(:exec).with(
              base_exec_env,
              described_class::MIGRATED_COMMANDS[command],
              gitaly_address,
              JSON.dump(gitaly_request),
              unsetenv_others: true,
              chdir: ROOT_PATH
            ).and_return(true)

            expect(subject.execute(command, args)).to be_truthy
          end
        end

        context 'with n relative config.git_trace_log_file' do
          let(:git_trace_log_file) { git_trace_log_file_relative }

          it 'returns true' do
            expect(Kernel).to receive(:exec).with(
              base_exec_env,
              described_class::MIGRATED_COMMANDS[command],
              gitaly_address,
              JSON.dump(gitaly_request),
              unsetenv_others: true,
              chdir: ROOT_PATH
            ).and_return(true)

            expect(subject.execute(command, args)).to be_truthy
          end
        end

        context 'with a valid config.git_trace_log_file' do
          let(:git_trace_log_file) { git_trace_log_file_valid }

          it 'returns true' do
            expect(Kernel).to receive(:exec).with(
              with_trace_exec_env,
              described_class::MIGRATED_COMMANDS[command],
              gitaly_address,
              JSON.dump(gitaly_request),
              unsetenv_others: true,
              chdir: ROOT_PATH
            ).and_return(true)

            expect(subject.execute(command, args)).to be_truthy
          end
        end
      end
    end
  end
end
