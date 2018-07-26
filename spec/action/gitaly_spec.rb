require_relative '../spec_helper'
require_relative '../../lib/action/gitaly'

describe Action::Gitaly do
  let(:git_trace_log_file_valid) { '/tmp/git_trace_performance.log' }
  let(:git_trace_log_file_invalid) { "/bleep-bop#{git_trace_log_file_valid}" }
  let(:git_trace_log_file_relative) { "..#{git_trace_log_file_valid}" }
  let(:key_id) { "key-#{rand(100) + 100}" }
  let(:gl_repository) { 'project-1' }
  let(:gl_username) { 'testuser' }
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
    described_class.new(key_id, gl_repository, gl_username, repository_path, gitaly)
  end

  describe '#execute' do
    let(:args) { [ repository_path ] }
    let(:base_exec_env) do
      {
        'HOME' => ENV['HOME'],
        'PATH' => ENV['PATH'],
        'LD_LIBRARY_PATH' => ENV['LD_LIBRARY_PATH'],
        'LANG' => ENV['LANG'],
        'GL_ID' => key_id,
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
        'gl_id' => key_id,
        'gl_username' => gl_username
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

        context 'with an relative config.git_trace_log_file' do
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
