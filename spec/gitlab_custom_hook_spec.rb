# coding: utf-8
require 'spec_helper'
require 'gitlab_custom_hook'

describe GitlabCustomHook do
  let(:original_root_path) { ROOT_PATH }
  let(:tmp_repo_path) { File.join(original_root_path, 'tmp', 'repo.git') }
  let(:tmp_root_path) { File.join(original_root_path, 'tmp') }
  let(:global_custom_hooks_path) { global_hook_path('custom_global_hooks') }
  let(:hook_ok) { File.join(original_root_path, 'spec', 'support', 'hook_ok') }
  let(:hook_fail) { File.join(original_root_path, 'spec', 'support', 'hook_fail') }
  let(:hook_gl_id) { File.join(original_root_path, 'spec', 'support', 'gl_id_test_hook') }

  let(:vars) { { "GL_ID" => "key_1" } }
  let(:old_value) { "old-value" }
  let(:new_value) { "new-value" }
  let(:ref_name) { "name/of/ref" }
  let(:changes) { "#{old_value} #{new_value} #{ref_name}\n" }

  let(:gitlab_custom_hook) { GitlabCustomHook.new(tmp_repo_path, 'key_1') }

  def hook_path(path)
    File.join(tmp_repo_path, path.split('/'))
  end

  def global_hook_path(path)
    File.join(tmp_root_path, path.split('/'))
  end

  def create_hook(path, which)
    FileUtils.ln_sf(which, hook_path(path))
  end

  # global hooks multiplexed
  def create_global_hooks_d(which, hook_name = 'hook')
    create_hook('hooks/pre-receive.d/' + hook_name, which)
    create_hook('hooks/update.d/' + hook_name, which)
    create_hook('hooks/post-receive.d/' + hook_name, which)
  end

  # repo hooks
  def create_repo_hooks(which)
    create_hook('custom_hooks/pre-receive', which)
    create_hook('custom_hooks/update', which)
    create_hook('custom_hooks/post-receive', which)
  end

  # repo hooks multiplexed
  def create_repo_hooks_d(which, hook_name = 'hook')
    create_hook('custom_hooks/pre-receive.d/' + hook_name, which)
    create_hook('custom_hooks/update.d/' + hook_name, which)
    create_hook('custom_hooks/post-receive.d/' + hook_name, which)
  end

  def cleanup_hook_setup
    FileUtils.rm_rf(File.join(tmp_repo_path))
    FileUtils.rm_rf(File.join(global_custom_hooks_path))
    FileUtils.rm_rf(File.join(tmp_root_path, 'hooks'))
    FileUtils.rm_f(File.join(tmp_root_path, 'config.yml'))
  end

  def expect_call_receive_hook(path)
    expect(gitlab_custom_hook)
      .to receive(:call_receive_hook)
      .with(hook_path(path), changes)
      .and_call_original
  end

  def expect_call_update_hook(path)
    expect(gitlab_custom_hook)
      .to receive(:system)
      .with(vars, hook_path(path), ref_name, old_value, new_value)
      .and_call_original
  end

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
    cleanup_hook_setup

    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks'))
    FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks'))

    ['pre-receive', 'update', 'post-receive'].each do |hook|
      FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks', "#{hook}.d"))
      FileUtils.mkdir_p(File.join(tmp_root_path, 'hooks', "#{hook}.d"))
    end

    FileUtils.symlink(File.join(tmp_root_path, 'hooks'), File.join(tmp_repo_path, 'hooks'))
    FileUtils.symlink(File.join(ROOT_PATH, 'config.yml.example'), File.join(tmp_root_path, 'config.yml'))

    stub_const('ROOT_PATH', tmp_root_path)
  end

  after do
    cleanup_hook_setup
  end

  context 'with gl_id_test_hook as repo hook' do
    before do
      create_repo_hooks(hook_gl_id)
    end

    context 'pre_receive hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      end
    end

    context 'post_receive hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
      end
    end

    context 'update hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      end
    end
  end

  context 'with gl_id_test_hook as global hook' do
    before do
      create_global_hooks_d(hook_gl_id)
    end

    context 'pre_receive hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      end
    end

    context 'post_receive hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
      end
    end

    context 'update hook' do
      it 'passes GL_ID variable to hook' do
        expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
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
      create_repo_hooks(hook_ok)
    end

    it "returns true" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having both successful repo and global hooks" do
    before do
      create_repo_hooks(hook_ok)
      create_global_hooks_d(hook_ok)
    end

    it "returns true" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having failing repo and successful global hooks" do
    before do
      create_repo_hooks_d(hook_fail)
      create_global_hooks_d(hook_ok)
    end

    it "returns false" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "only executes the global hook" do
      expect_call_receive_hook("custom_hooks/pre-receive.d/hook")
      expect_call_update_hook("custom_hooks/update.d/hook")
      expect_call_receive_hook("custom_hooks/post-receive.d/hook")

      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  context "having successful repo but failing global hooks" do
    before do
      create_repo_hooks_d(hook_ok)
      create_global_hooks_d(hook_fail)
    end

    it "returns false" do
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "executes the relevant hooks" do
      expect_call_receive_hook("hooks/pre-receive.d/hook")
      expect_call_receive_hook("custom_hooks/pre-receive.d/hook")
      expect_call_update_hook("hooks/update.d/hook")
      expect_call_update_hook("custom_hooks/update.d/hook")
      expect_call_receive_hook("hooks/post-receive.d/hook")
      expect_call_receive_hook("custom_hooks/post-receive.d/hook")

      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  context "executing hooks in expected order" do
    before do
      create_repo_hooks_d(hook_ok, '01-test')
      create_repo_hooks_d(hook_ok, '02-test')
      create_global_hooks_d(hook_ok, '03-test')
      create_global_hooks_d(hook_ok, '04-test')
    end

    it "executes hooks in order" do
      expect_call_receive_hook("custom_hooks/pre-receive.d/01-test").ordered
      expect_call_receive_hook("custom_hooks/pre-receive.d/02-test").ordered
      expect_call_receive_hook("hooks/pre-receive.d/03-test").ordered
      expect_call_receive_hook("hooks/pre-receive.d/04-test").ordered

      expect_call_update_hook("custom_hooks/update.d/01-test").ordered
      expect_call_update_hook("custom_hooks/update.d/02-test").ordered
      expect_call_update_hook("hooks/update.d/03-test").ordered
      expect_call_update_hook("hooks/update.d/04-test").ordered

      expect_call_receive_hook("custom_hooks/post-receive.d/01-test").ordered
      expect_call_receive_hook("custom_hooks/post-receive.d/02-test").ordered
      expect_call_receive_hook("hooks/post-receive.d/03-test").ordered
      expect_call_receive_hook("hooks/post-receive.d/04-test").ordered

      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  context "when the custom_hooks_dir config option is set" do
    before do
      allow(gitlab_custom_hook.config).to receive(:custom_hooks_dir).and_return(global_custom_hooks_path)

      FileUtils.mkdir_p(File.join(global_custom_hooks_path, "pre-receive.d"))
      FileUtils.ln_sf(hook_ok, File.join(global_custom_hooks_path, "pre-receive.d", "hook"))

      create_global_hooks_d(hook_fail)
    end

    it "finds hooks in that directory" do
      expect(gitlab_custom_hook)
        .to receive(:call_receive_hook)
        .with(global_hook_path("custom_global_hooks/pre-receive.d/hook"), changes)
        .and_call_original

      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
    end

    it "does not execute hooks in the default location" do
      expect(gitlab_custom_hook)
        .not_to receive(:call_receive_hook)
        .with("hooks/pre-receive.d/hook", changes)
        .and_call_original

      gitlab_custom_hook.pre_receive(changes)
    end
  end
end
