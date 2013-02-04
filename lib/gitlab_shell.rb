require 'open3'
require 'net/http'

require_relative 'gitlab_config'

class GitlabShell
  attr_accessor :username, :repo_name, :git_cmd, :repos_path

  def initialize
    @username = ARGV.shift
    @origin_cmd = ENV['SSH_ORIGINAL_COMMAND']
    @repos_path = GitlabConfig.new.repos_path
  end

  def exec
    if @origin_cmd
      parse_cmd

      if git_cmds.include?(@git_cmd)
        ENV['GL_USER'] = @username

        if validate_access
          process_cmd
        end
      else
        puts 'Not allowed command'
      end
    else
      puts "Welcome #{@username}!"
    end
  end

  protected

  def parse_cmd
    args = @origin_cmd.split(' ')
    @git_cmd = args.shift
    @repo_name = args.shift
  end

  def git_cmds
    %w(git-upload-pack git-receive-pack git-upload-archive)
  end

  def process_cmd
    repo_full_path = File.join(repos_path, repo_name)
    system("#{@git_cmd} #{repo_full_path}")
  end

  def validate_access
    @ref_name = 'master' # just hardcode it cause we dont know ref
    project_name = @repo_name.gsub("'", "")
    project_name = project_name.gsub(/\.git$/, "")
    url = "http://127.0.0.1:3000/api/v3/allowed?project=#{project_name}&username=#{@username}&action=#{@git_cmd}&ref=#{@ref_name}"
    resp = Net::HTTP.get_response(URI.parse(url))
    resp.code == '200' && resp.body == 'true'
  end
end
