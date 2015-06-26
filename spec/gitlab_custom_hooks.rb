require 'support/publicize_methods.rb'
require 'spec_helper'
require 'gitlab_custom_hook'

describe GitlabCustomHook do
  let(:repository_path) { '/home/git/repositories' }
  let(:repo_name) { 'dzaporozhets/gitlab-ci' }
  let(:repo_path) { File.join(repository_path, repo_name) + '.git' }
  let(:gitlab_custom_hooks) { GitlabCustomHook.new }
  let(:local_pre_receive) { File.join(repo_path, 'custom_hook', 'pre-receive') }
  let(:local_post_receive) { File.join(repo_path, 'custom_hook', 'post-receive') }
  let(:global_pre_receive) { File.join(ROOT_PATH, 'custom_hook', 'pre-receive') }
  let(:global_post_receive) { File.join(ROOT_PATH, 'custom_hook', 'post-receive') }

  describe 'select hook file' do
    context 'with nil arguments' do
      it 'returns nil' do
        GitlabCustomHook.publicize_methods do
          expect(gitlab_custom_hooks.hook_file(nil, repo_path)).to eq(nil)
          expect(gitlab_custom_hooks.hook_file('post-receive', nil)).to eq(nil)
          expect(gitlab_custom_hooks.hook_file(nil, nil)).to eq(nil)
        end
      end
    end

    context 'when the local hook path exists' do
      before do
        File.stub(:exist?).with(local_post_receive).and_return(true)
        File.stub(:exist?).with(local_pre_receive).and_return(true)
      end
      context 'when the global hook path exists' do
        before do
          File.stub(:exist?).with(global_post_receive).and_return(true)
          File.stub(:exist?).with(global_pre_receive).and_return(true)
        end
        it 'returns the local hook file' do
          GitlabCustomHook.publicize_methods do
            expect(gitlab_custom_hooks.hook_file('pre-receive', repo_path)).to eq(local_pre_receive)
            expect(gitlab_custom_hooks.hook_file('post-receive', repo_path)).to eq(local_post_receive)
          end
        end
      end
      context 'when the global hook path does not exist' do
        before do
          File.stub(:exist?).with(global_post_receive).and_return(false)
          File.stub(:exist?).with(global_pre_receive).and_return(false)
        end
        it 'returns the local hook file' do
          GitlabCustomHook.publicize_methods do
            expect(gitlab_custom_hooks.hook_file('pre-receive', repo_path)).to eq(local_pre_receive)
            expect(gitlab_custom_hooks.hook_file('post-receive', repo_path)).to eq(local_post_receive)
          end
        end
      end
    end
    context 'when the local hook path does not exists' do
      before do
        File.stub(:exist?).with(local_post_receive).and_return(false)
        File.stub(:exist?).with(local_pre_receive).and_return(false)
      end
      context 'when the global hook path exists' do
        before do
          File.stub(:exist?).with(global_post_receive).and_return(true)
          File.stub(:exist?).with(global_pre_receive).and_return(true)
        end
        it 'returns the local hook file' do
          GitlabCustomHook.publicize_methods do
            expect(gitlab_custom_hooks.hook_file('pre-receive', repo_path)).to eq(global_pre_receive)
            expect(gitlab_custom_hooks.hook_file('post-receive', repo_path)).to eq(global_post_receive)
          end
        end
      end
      context 'when the global hook path does not exist' do
        before do
          File.stub(:exist?).with(global_post_receive).and_return(false)
          File.stub(:exist?).with(global_pre_receive).and_return(false)
        end
        it 'returns the local hook file' do
          GitlabCustomHook.publicize_methods do
            expect(gitlab_custom_hooks.hook_file('pre-receive', repo_path)).to eq(nil)
            expect(gitlab_custom_hooks.hook_file('post-receive', repo_path)).to eq(nil)
          end
        end
      end
    end
  end
end
