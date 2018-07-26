require_relative '../action'
require_relative '../gitlab_logger'

module Action
  class GitLFSAuthenticate < Base
    def initialize(key_id, repo_name)
      @key_id = key_id
      @repo_name = repo_name
    end

    def execute(_, _)
      GitlabMetrics.measure('lfs-authenticate') do
        $logger.info('Processing LFS authentication', user: user.log_username)
        lfs_access = api.lfs_authenticate(key_id, repo_name)
        return unless lfs_access

        puts lfs_access.authentication_payload
      end
      true
    end

    private

    attr_reader :key_id, :repo_name
  end
end
