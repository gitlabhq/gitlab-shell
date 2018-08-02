require_relative '../action'
require_relative '../gitlab_logger'

module Action
  class GitLFSAuthenticate < Base
    def initialize(actor, repo_name)
      @actor = actor
      @repo_name = repo_name
    end

    def execute(_, _)
      GitlabMetrics.measure('lfs-authenticate') do
        $logger.info('Processing LFS authentication', user: actor.log_username)
        lfs_access = api.lfs_authenticate(actor, repo_name)
        return unless lfs_access

        puts lfs_access.authentication_payload
      end
      true
    end

    private

    attr_reader :actor, :repo_name
  end
end
