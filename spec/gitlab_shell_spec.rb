require_relative 'spec_helper'
require_relative '../lib/gitlab_shell'
require_relative '../lib/action'

describe GitlabShell do
  before do
    $logger = double('logger').as_null_object
    FileUtils.mkdir_p(tmp_repos_path)
  end

  after { FileUtils.rm_rf(tmp_repos_path) }

  subject { described_class.new(who) }

  let(:who) { 'key-1' }
  let(:audit_usernames) { true }
  let(:actor) { Actor.new_from(who, audit_usernames: audit_usernames) }
  let(:tmp_repos_path) { File.join(ROOT_PATH, 'tmp', 'repositories') }
  let(:repo_name) { 'gitlab-ci.git' }
  let(:repo_path) { File.join(tmp_repos_path, repo_name) }
  let(:gl_repository) { 'project-1' }
  let(:gl_username) { 'testuser' }
  let(:git_protocol) { 'version=2' }

  let(:api) { double(GitlabNet) }
  let(:config) { double(GitlabConfig) }

  let(:gitaly_action) { Action::Gitaly.new(
                          actor,
                          gl_repository,
                          gl_username,
                          git_protocol,
                          repo_path,
                          { 'repository' => { 'relative_path' => repo_name, 'storage_name' => 'default' } , 'address' => 'unix:gitaly.socket' })
                      }
  let(:api_2fa_recovery_action) { Action::API2FARecovery.new(actor) }
  let(:git_lfs_authenticate_action) { Action::GitLFSAuthenticate.new(actor, repo_name) }

  before do
    allow(GitlabConfig).to receive(:new).and_return(config)
    allow(config).to receive(:audit_usernames).and_return(audit_usernames)

    allow(Actor).to receive(:new_from).with(who, audit_usernames: audit_usernames).and_return(actor)

    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(actor).and_return('username' => gl_username)
  end

  describe '#exec' do
    context "when we don't have a valid user" do
      before do
        allow(api).to receive(:discover).with(actor).and_return(nil)
      end

      it 'prints Welcome.. and returns true' do
        expect {
          expect(subject.exec(nil)).to be_truthy
        }.to output("Welcome to GitLab, Anonymous!\n").to_stdout
      end
    end

    context 'when we have a valid user' do
      context 'when origin_cmd is nil' do
        it 'prints Welcome.. and returns true' do
          expect {
            expect(subject.exec(nil)).to be_truthy
          }.to output("Welcome to GitLab, @testuser!\n").to_stdout
        end
      end

      context 'when origin_cmd is empty' do
        it 'prints Welcome.. and returns true' do
          expect {
            expect(subject.exec('')).to be_truthy
          }.to output("Welcome to GitLab, @testuser!\n").to_stdout
        end
      end
    end

    context 'when origin_cmd is invalid' do
      it 'prints a message to stderr and returns false' do
        expect {
          expect(subject.exec("git-invalid-command #{repo_name}")).to be_falsey
        }.to output("GitLab: Disallowed command\n").to_stderr
      end
    end

    context 'when origin_cmd is valid, but incomplete' do
      it 'prints a message to stderr and returns false' do
        expect {
          expect(subject.exec('git-upload-pack')).to be_falsey
        }.to output("GitLab: Disallowed command\n").to_stderr
      end
    end

    context 'when origin_cmd is git-lfs-authenticate' do
      context 'but incomplete' do
        it 'prints a message to stderr and returns false' do
          expect {
            expect(subject.exec('git-lfs-authenticate')).to be_falsey
          }.to output("GitLab: Disallowed command\n").to_stderr
        end
      end

      context 'but invalid' do
        it 'prints a message to stderr and returns false' do
          expect {
            expect(subject.exec("git-lfs-authenticate #{repo_name} invalid")).to be_falsey
          }.to output("GitLab: Disallowed command\n").to_stderr
        end
      end
    end

    context 'when origin_cmd is 2fa_recovery_codes' do
      let(:origin_cmd) { '2fa_recovery_codes' }
      let(:git_access) { '2fa_recovery_codes' }

      before do
        expect(Action::API2FARecovery).to receive(:new).with(actor).and_return(api_2fa_recovery_action)
      end

      it 'returns true' do
        expect(api_2fa_recovery_action).to receive(:execute).with('2fa_recovery_codes', %w{ 2fa_recovery_codes }).and_return(true)
        expect(subject.exec(origin_cmd)).to be_truthy
      end
    end

    context 'when access to the repo is denied' do
      before do
        expect(api).to receive(:check_access).with('git-upload-pack', nil, repo_name, actor, '_any').and_raise(AccessDeniedError, 'Sorry, access denied')
      end

      it 'prints a message to stderr and returns false' do
        expect($stderr).to receive(:puts).with('GitLab: Sorry, access denied')
        expect(subject.exec("git-upload-pack #{repo_name}")).to be_falsey
      end
    end

    context 'when the API is unavailable' do
      before do
        expect(api).to receive(:check_access).with('git-upload-pack', nil, repo_name, actor, '_any').and_raise(GitlabNet::ApiUnreachableError)
      end

      it 'prints a message to stderr and returns false' do
        expect($stderr).to receive(:puts).with('GitLab: Failed to authorize your Git request: internal API unreachable')
        expect(subject.exec("git-upload-pack #{repo_name}")).to be_falsey
      end
    end

    context 'when access has been verified OK' do
      before do
        expect(api).to receive(:check_access).with(git_access, nil, repo_name, actor, '_any').and_return(gitaly_action)
      end

      context 'when origin_cmd is git-upload-pack' do
        let(:origin_cmd) { 'git-upload-pack' }
        let(:git_access) { 'git-upload-pack' }

        it 'returns true' do
          expect(gitaly_action).to receive(:execute).with('git-upload-pack', %W{git-upload-pack #{repo_name}}).and_return(true)
          expect(subject.exec("#{origin_cmd} #{repo_name}")).to be_truthy
        end

        context 'but repo path is invalid' do
          it 'prints a message to stderr and returns false' do
            expect(gitaly_action).to receive(:execute).with('git-upload-pack', %W{git-upload-pack #{repo_name}}).and_raise(InvalidRepositoryPathError)
            expect($stderr).to receive(:puts).with('GitLab: Invalid repository path')
            expect(subject.exec("#{origin_cmd} #{repo_name}")).to be_falsey
          end
        end

        context "but we're using an old git version for Windows 2.14" do
          it 'returns true' do
            expect(gitaly_action).to receive(:execute).with('git-upload-pack', %W{git-upload-pack #{repo_name}}).and_return(true)
            expect(subject.exec("git upload-pack #{repo_name}")).to be_truthy #NOTE: 'git upload-pack' vs. 'git-upload-pack'
          end
        end
      end

      context 'when origin_cmd is git-lfs-authenticate' do
        let(:origin_cmd) { 'git-lfs-authenticate' }
        let(:lfs_access) { double(GitlabLfsAuthentication, authentication_payload: fake_payload)}

        before do
          expect(Action::GitLFSAuthenticate).to receive(:new).with(actor, repo_name).and_return(git_lfs_authenticate_action)
        end

        context 'upload' do
          let(:git_access) { 'git-receive-pack' }

          it 'returns true' do
            expect(git_lfs_authenticate_action).to receive(:execute).with('git-lfs-authenticate', %w{ git-lfs-authenticate gitlab-ci.git upload }).and_return(true)
            expect(subject.exec("#{origin_cmd} #{repo_name} upload")).to be_truthy
          end
        end

        context 'download' do
          let(:git_access) { 'git-upload-pack' }

          it 'returns true' do
            expect(git_lfs_authenticate_action).to receive(:execute).with('git-lfs-authenticate', %w{ git-lfs-authenticate gitlab-ci.git download }).and_return(true)
            expect(subject.exec("#{origin_cmd} #{repo_name} download")).to be_truthy
          end

          context 'for old git-lfs clients' do
            it 'returns true' do
              expect(git_lfs_authenticate_action).to receive(:execute).with('git-lfs-authenticate', %w{ git-lfs-authenticate gitlab-ci.git download long_oid }).and_return(true)
              expect(subject.exec("#{origin_cmd} #{repo_name} download long_oid")).to be_truthy
            end
          end
        end
      end
    end
  end
end
