ROOT_PATH = ENV.fetch('GITLAB_SHELL_DIR', File.expand_path('..', __dir__))

# We are transitioning parts of gitlab-shell into the gitaly project. In
# gitaly, GITALY_EMBEDDED will be true.
GITALY_EMBEDDED = false

require_relative 'gitlab_config'
