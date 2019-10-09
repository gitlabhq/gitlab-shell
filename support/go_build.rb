# Helper functions to build go code in gitlab-shell

require 'fileutils'

# Determine the root of the gitlab-shell directory
ROOT_PATH = ENV.fetch('GITLAB_SHELL_DIR', File.expand_path('..', __dir__))

module GoBuild
  GO_DIR = File.join(ROOT_PATH, 'go')
  BUILD_DIR = File.join(ROOT_PATH, 'go_build')

  GO_ENV = {
    # $GOBIN controls where 'go install' puts binaries. Prior to go mod,
    # this was $GOPATH/bin.
    'GOBIN' => File.join(BUILD_DIR, 'bin'),
    # Force the use of go mod, even if $GOPATH is set.
    'GO111MODULE' => 'on',
    # Downloading dependencies via proxy.golang.org is faster and more
    # reliable than downloading from canonical sources. We need this
    # environment variable because as of Go 1.12, the default is still to
    # download from canonical sources.
    'GOPROXY' => 'https://proxy.golang.org'
  }.freeze

  def ensure_build_dir_exists
    FileUtils.mkdir_p(BUILD_DIR)
  end

  def run!(env, cmd, options = {})
    raise "env must be a hash" unless env.is_a?(Hash)
    raise "cmd must be an array" unless cmd.is_a?(Array)

    unless system(env, *cmd, options)
      abort "command failed: #{env.inspect} #{cmd.join(' ')}"
    end
  end
end
