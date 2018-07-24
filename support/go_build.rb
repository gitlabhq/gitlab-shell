# Helper functions to build go code in gitlab-shell

require 'fileutils'

# This will set the ROOT_PATH variable
require_relative '../lib/gitlab_init'

module GoBuild
  GO_DIR = 'go'.freeze
  BUILD_DIR = File.join(ROOT_PATH, 'go_build')
  GO_PACKAGE = File.join('gitlab.com/gitlab-org/gitlab-shell', GO_DIR)

  GO_ENV = {
    'GOPATH' => BUILD_DIR,
    'GO15VENDOREXPERIMENT' => '1'
  }.freeze

  def create_fresh_build_dir
    FileUtils.rm_rf(BUILD_DIR)
    build_source_dir = File.join(BUILD_DIR, 'src', GO_PACKAGE)
    FileUtils.mkdir_p(build_source_dir)
    FileUtils.cp_r(File.join(ROOT_PATH, GO_DIR, '.'), build_source_dir)
  end

  def run!(env, cmd, options = {})
    raise "env must be a hash" unless env.is_a?(Hash)
    raise "cmd must be an array" unless cmd.is_a?(Array)

    unless system(env, *cmd, options)
      abort "command failed: #{env.inspect} #{cmd.join(' ')}"
    end
  end
end
