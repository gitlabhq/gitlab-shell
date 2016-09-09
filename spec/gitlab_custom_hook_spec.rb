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

  before do
    FileUtils.mkdir_p(File.join(tmp_repo_path, 'custom_hooks'))
    FileUtils.mkdir_p(File.join(tmp_root_path, 'custom_hooks'))
  end

  after do
    FileUtils.rm_rf(File.join(tmp_repo_path, 'custom_hooks'))
    FileUtils.rm_rf(File.join(tmp_root_path, 'custom_hooks'))
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
      stub_const("ROOT_PATH", tmp_root_path)
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having only ok repo hooks" do
    before do
      create_hooks(tmp_repo_path, hook_ok)
    end

    it "returns true" do
      stub_const("ROOT_PATH", tmp_root_path)
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having both ok repo and global hooks" do
    before do
      create_hooks(tmp_repo_path, hook_ok)
      create_hooks(tmp_root_path, hook_ok)
    end

    it "returns true" do
      stub_const("ROOT_PATH", tmp_root_path)
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(true)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(true)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(true)
    end
  end

  context "having failing repo and ok global hooks" do
    before do
      create_hooks(tmp_repo_path, hook_fail)
      create_hooks(tmp_root_path, hook_ok)
    end

    it "returns false" do
      stub_const("ROOT_PATH", tmp_root_path)
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "only executes the repo hook" do
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "pre-receive"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_repo_path, "update"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "post-receive"), changes)
        .and_call_original

      stub_const("ROOT_PATH", tmp_root_path)
      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  context "having ok repo but failing global hooks" do
    before do
      create_hooks(tmp_repo_path, hook_ok)
      create_hooks(tmp_root_path, hook_fail)
    end

    it "returns false" do
      stub_const("ROOT_PATH", tmp_root_path)
      expect(gitlab_custom_hook.pre_receive(changes)).to eq(false)
      expect(gitlab_custom_hook.update(ref_name, old_value, new_value)).to eq(false)
      expect(gitlab_custom_hook.post_receive(changes)).to eq(false)
    end

    it "executes the relevant hooks" do
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "pre-receive"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_root_path, "pre-receive"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_repo_path, "update"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:system)
        .with(vars, hook_path(tmp_root_path, "update"), ref_name, old_value, new_value)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_repo_path, "post-receive"), changes)
        .and_call_original
      expect(gitlab_custom_hook).to receive(:call_receive_hook)
        .with(hook_path(tmp_root_path, "post-receive"), changes)
        .and_call_original

      stub_const("ROOT_PATH", tmp_root_path)
      gitlab_custom_hook.pre_receive(changes)
      gitlab_custom_hook.update(ref_name, old_value, new_value)
      gitlab_custom_hook.post_receive(changes)
    end
  end

  def hook_path(path, name)
    File.join(path, 'custom_hooks', name)
  end

  def create_hook(path, name, which)
    FileUtils.ln_sf(which, hook_path(path, name))
  end

  def create_hooks(path, which)
    create_hook(path, 'pre-receive', which)
    create_hook(path, 'update', which)
    create_hook(path, 'post-receive', which)
  end
end
