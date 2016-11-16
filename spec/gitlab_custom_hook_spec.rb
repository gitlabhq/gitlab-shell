# coding: utf-8
require 'spec_helper'
require 'gitlab_custom_hook'

describe GitlabCustomHook do
  let(:tmp_repo_path) { File.join(ROOT_PATH, 'tmp', 'repo.git') }
  let(:tmp_root_path) { File.join(ROOT_PATH, 'tmp') }
  let(:hook_ok) { File.join(ROOT_PATH, 'spec', 'support', 'hook_ok') }
  let(:hook_fail) { File.join(ROOT_PATH, 'spec', 'support', 'hook_fail') }

  let(:vars) { {"GL_ID" => "key_1"} }
  let(:old_value) { "old-value" }
  let(:new_value) { "new-value" }
  let(:ref_name) { "name/of/ref" }
  let(:changes) { "#{old_value} #{new_value} #{ref_name}\n" }

  let(:gitlab_custom_hook) { GitlabCustomHook.new(tmp_repo_path, 'key_1') }

  # setup paths
  # <repository>.git/hooks/ - symlink to gitlab-shell/hooks global dir
  # <repository>.git/hooks/<hook_name> - executed by git itself, this is gitlab-shell/hooks/<hook_name>
  # <repository>.git/hooks/<hook_name>.d/* - global hooks: all executable files (minus editor backup files)
  # <repository>.git/custom_hooks/<hook_name> - per project hook (this is already existing behavior)
  # <repository>.git/custom_hooks/<hook_name>.d/* - per project hooks
  #
  # custom hooks are invoked in such way that first failure prevents other scripts being ran
  # as global scripts are ran first, failing global skips repo hooks

  before do
    FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks'))
    FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks', 'update.d'))
    FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks', 'pre-receive.d'))
    FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks', 'post-receive.d'))

    FileUtils.symlink(File.join(tmp_root_path, 'hooks'), File.join(tmp_repo_path, 'hooks'))
    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks'))
    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks', 'update.d'))
    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks', 'pre-receive.d'))
    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks', 'post-receive.d'))
  end

  after do
    FileUtils.rm_rf(File.join(tmp_repo_path, 'custom_hooks'))
    FileUtils.rm_rf(File.join(tmp_repo_path, 'hooks'))
    FileUtils.rm_rf(File.join(tmp_repo_path, 'hooks.d'))
    FileUtils.rm_rf(File.join(tmp_root_path, 'hooks'))
  end

  context 'with gl_id_test_hook' do
    let(:hook_path) { File.join(ROOT_PATH, 'spec/support/gl_id_test_hook') }

    context 'pre_receive hook' do
      it 'passes GL_ID variable to hook' do
        allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

        expect(gitlab_custom_hook.pre_receive(changes)).to be_true
      end
    end

    context 'post_receive hook' do
      it 'passes GL_ID variable to hook' do
        allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

        expect(gitlab_custom_hook.post_receive(changes)).to be_true
      end
    end

    context 'update hook' do
      it 'passes GL_ID variable to hook' do
        allow(gitlab_custom_hook).to receive(:hook_file).and_return(hook_path)

        expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to be_true
      end
    end
  end

  context "having no hooks" do
    it "returns true" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having only successful repo hooks" do
    before do
      create_repo_hooks(tmp_repo_path, hook_ok)
    end

    it "returns true" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having both successful repo and global hooks" do
    before do
      create_repo_hooks(tmp_repo_path, hook_ok)
      create_global_hooks_d(tmp_root_path, hook_ok)
    end

    it "returns true" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having failing repo and successful global hooks" do
    before do
      create_repo_hooks_d(tmp_repo_path, hook_fail)
      create_global_hooks_d(tmp_repo_path, hook_ok)
    end

    it "returns false" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "only executes the global hook" do
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "custom_hooks/pre-receive.d/hook"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_repo_path, "custom_hooks/update.d/hook"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "custom_hooks/post-receive.d/hook"), changes)
        .and_call_original

      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  context "having successful repo but failing global hooks" do
    before do
      create_repo_hooks_d(tmp_repo_path, hook_ok)
      create_global_hooks_d(tmp_repo_path, hook_fail)
    end

    it "returns false" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "executes the relevant hooks" do
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "hooks/pre-receive.d/hook"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "custom_hooks/pre-receive.d/hook"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_repo_path, "hooks/update.d/hook"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_repo_path, "custom_hooks/update.d/hook"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "hooks/post-receive.d/hook"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "custom_hooks/post-receive.d/hook"), changes)
        .and_call_original

      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  def hook_path(repo_path, path)
    File.join(repo_path, path.split('/'))
  end

  def create_hook(repo_path, path, which)
    FileUtils.ln_sf(which, hook_path(repo_path, path))
  end

  # global hooks multiplexed
  def create_global_hooks_d(path, which)
    create_hook(path, 'hooks/pre-receive.d/hook', which)
    create_hook(path, 'hooks/update.d/hook', which)
    create_hook(path, 'hooks/post-receive.d/hook', which)
  end

  # repo hooks
  def create_repo_hooks(path, which)
    create_hook(path, 'custom_hooks/pre-receive', which)
    create_hook(path, 'custom_hooks/update', which)
    create_hook(path, 'custom_hooks/post-receive', which)
  end

  # repo hooks multiplexed
  def create_repo_hooks_d(path, which)
    create_hook(path, 'custom_hooks/pre-receive.d/hook', which)
    create_hook(path, 'custom_hooks/update.d/hook', which)
    create_hook(path, 'custom_hooks/post-receive.d/hook', which)
  end
end
