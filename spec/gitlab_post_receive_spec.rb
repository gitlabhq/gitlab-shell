require 'spec_helper'
require 'gitlab_post_receive'

describe GitlabPostReceive do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name)   { 'dzaporozhets/gitlab-ci' }
  let(:actor)   { 'key-123' }
  let(:changes) { "123456 789012 refs/heads/tést\n654321 210987 refs/tags/tag" }
  let(:wrongly_encoded_changes) { changes.encode("ISO-8859-1").force_encoding("UTF-8") }
  let(:base64_changes) { Base64.encode64(wrongly_encoded_changes) }
  let(:repo_path)  { File.join(repository_path, repo_name) + ".git" }
  let(:gitlab_post_receive) { GitlabPostReceive.new(repo_path, actor, wrongly_encoded_changes) }
  let(:message) { "test " * 10 + "message " * 10 }

  before do
    GitlabConfig.any_instance.stub(repos_path: repository_path)
    GitlabNet.any_instance.stub(broadcast_message: { "message" => message })
  end

  describe "#exec" do

    before do
      GitlabConfig.any_instance.stub(redis_command: %w(env -i redis-cli))
      allow(gitlab_post_receive).to receive(:system).and_return(true)
    end

    it "prints the broadcast message" do
      expect(gitlab_post_receive).to receive(:puts).ordered
      expect(gitlab_post_receive).to receive(:puts).with(
        "========================================================================"
      ).ordered
      expect(gitlab_post_receive).to receive(:puts).ordered

      expect(gitlab_post_receive).to receive(:puts).with(
        "   test test test test test test test test test test message message"
      ).ordered
      expect(gitlab_post_receive).to receive(:puts).with(
        "    message message message message message message message message"
      ).ordered

      expect(gitlab_post_receive).to receive(:puts).ordered      
      expect(gitlab_post_receive).to receive(:puts).with(
        "========================================================================"
      ).ordered

      gitlab_post_receive.exec
    end

    it "pushes a Sidekiq job onto the queue" do
      expect(gitlab_post_receive).to receive(:system).with(
        *[
          *%w(env -i redis-cli rpush resque:gitlab:queue:post_receive), 
          %Q/{"class":"PostReceive","args":["#{repo_path}","#{actor}",#{base64_changes.inspect}]}/,
          { err: "/dev/null", out: "/dev/null" }
        ]
      ).and_return(true)

      gitlab_post_receive.exec
    end

    context "when the redis command succeeds" do

      before do
        allow(gitlab_post_receive).to receive(:system).and_return(true)
      end

      it "returns true" do
        expect(gitlab_post_receive.exec).to eq(true)
      end
    end

    context "when the redis command fails" do

      before do
        allow(gitlab_post_receive).to receive(:system).and_return(false)
        allow($?).to receive(:exitstatus).and_return(nil)
      end

      it "returns false" do
        expect(gitlab_post_receive.exec).to eq(false)
      end
    end
  end
end
