# coding: utf-8
require 'spec_helper'
require 'pry'
require 'gitlab_custom_hook'

describe GitlabCustomHook do
  let(:gitlab_custom_hook) { GitlabCustomHook.new('repo_path', 'key_1') }
  let(:hook_path) { File.join(ROOT_PATH, 'spec/support/gl_id_test_hook') }

  context 'pre_receive hook' do
    it 'passes GL_ID variable to hook' do
      allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

      expect(gitlab_custom_hook.pre_receive('changes')).to be_true
    end
  end

  context 'post_receive hook' do
    it 'passes GL_ID variable to hook' do
      allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

      expect(gitlab_custom_hook.post_receive('changes')).to be_true
    end
  end

  context 'update hook' do
    it 'passes GL_ID variable to hook' do
      allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

      expect(gitlab_custom_hook.update('master', '', '')).to be_true
    end
  end
end
