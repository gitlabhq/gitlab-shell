require 'spec_helper'
require 'gitlab_merge_request_hook'

describe GitlabMergeRequestHook do
  let(:changes)         { "123456 789012 refs/heads/#{branch_name}" }
  let(:config)          { GitlabConfig.new }
  let(:repository_path) { '/home/git/repositories' }
  let(:repo_name)       { 'dzaporozhets/gitlab-ci' }
  let(:repo_path)       { File.join(repository_path, repo_name) + '.git' }
  let(:hook)            { described_class.new(repo_path, config, changes) }

  before do
    GitlabConfig.any_instance.stub(repos_path: repository_path)
    GitlabConfig.any_instance.stub(gitlab_url: 'http://mygitlab.com')
  end

  describe '#exec' do
    subject { hook.exec }

    context 'pushing on master' do
      let(:branch_name) { 'master' }

      it 'does not print the merge request url' do
        expect(hook).not_to receive(:puts)
        subject
      end
    end

    context 'pushing on another branch' do
      let(:branch_name) { 'my_branch' }

      it 'prints the branch url' do
        expect(hook).to receive(:puts).ordered

        expect(hook).to receive(:puts).with(
          'To open a merge request for my_branch, enter in:'
        )

        expect(hook).to receive(:puts).with(
          "\thttp://mygitlab.com/dzaporozhets/gitlab-ci/merge_requests/new?" \
            'merge_request[source_branch]=my_branch&' \
            'merge_request[target_branch]=master'
        )

        expect(hook).to receive(:puts).ordered

        subject
      end

      context 'pushing only tags' do
        let(:changes) { '654321 210987 refs/tags/tag' }

        it 'does not print the merge request url' do
          expect(hook).not_to receive(:puts)
          subject
        end
      end
    end
  end
end
