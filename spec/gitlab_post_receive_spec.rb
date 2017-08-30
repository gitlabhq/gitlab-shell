# coding: utf-8
require 'spec_helper'
require 'gitlab_post_receive'

describe GitlabPostReceive do
  let(:repository_path) { "/home/git/repositories" }
  let(:repo_name) { 'dzaporozhets/gitlab-ci' }
  let(:actor) { 'key-123' }
  let(:changes) { "123456 789012 refs/heads/tést\n654321 210987 refs/tags/tag" }
  let(:wrongly_encoded_changes) { changes.encode("ISO-8859-1").force_encoding("UTF-8") }
  let(:base64_changes) { Base64.encode64(wrongly_encoded_changes) }
  let(:repo_path) { File.join(repository_path, repo_name) + ".git" }
  let(:gl_repository) { "project-1" }
  let(:gitlab_post_receive) { GitlabPostReceive.new(gl_repository, repo_path, actor, wrongly_encoded_changes) }
  let(:broadcast_message) { "test " * 10 + "message " * 10 }
  let(:redis_client) { double('redis_client') }
  let(:enqueued_at) { Time.new(2016, 6, 23, 6, 59) }
  let(:new_merge_request_urls) do
    [{
      'branch_name' => 'new_branch',
      'url' => 'http://localhost/dzaporozhets/gitlab-ci/merge_requests/new?merge_request%5Bsource_branch%5D=new_branch',
      'new_merge_request' => true
    }]
  end
  let(:existing_merge_request_urls) do
    [{
      'branch_name' => 'feature_branch',
      'url' => 'http://localhost/dzaporozhets/gitlab-ci/merge_requests/1',
      'new_merge_request' => false
    }]
  end

  before do
    $logger = double('logger').as_null_object # Global vars are bad
    GitlabConfig.any_instance.stub(repos_path: repository_path)
  end

  describe "#exec" do
    context 'when the new post_receive API endpoint is not available' do
      before do
        GitlabNet.any_instance.stub(broadcast_message: { })
        GitlabNet.any_instance.stub(:merge_request_urls).with(gl_repository, repo_path, wrongly_encoded_changes) { [] }
        GitlabNet.any_instance.stub(notify_post_receive: true)

        allow_any_instance_of(GitlabNet).to receive(:post_receive).and_raise(GitlabNet::NotFound)
        allow_any_instance_of(GitlabNet).to receive(:redis_client).and_return(redis_client)
        allow_any_instance_of(GitlabReferenceCounter).to receive(:redis_client).and_return(redis_client)
        allow(redis_client).to receive(:get).and_return(1)
        allow(redis_client).to receive(:incr).and_return(true)
        allow(redis_client).to receive(:decr).and_return(0)
        allow(redis_client).to receive(:rpush).and_return(true)
        expect(Time).to receive(:now).and_return(enqueued_at)
      end

      context 'Without broad cast message' do
        context 'pushing new branch' do
          before do
            GitlabNet.any_instance.stub(:merge_request_urls).with(gl_repository, repo_path, wrongly_encoded_changes) do
              new_merge_request_urls
            end
          end

          it "prints the new merge request url" do
            assert_new_mr_printed(gitlab_post_receive)

            gitlab_post_receive.exec
          end
        end

        context 'pushing existing branch with merge request created' do
          before do
            GitlabNet.any_instance.stub(:merge_request_urls).with(gl_repository, repo_path, wrongly_encoded_changes) do
              existing_merge_request_urls
            end
          end

          it "prints the view merge request url" do
            assert_existing_mr_printed(gitlab_post_receive)

            gitlab_post_receive.exec
          end
        end
      end

      context 'show broadcast message and merge request link' do
        before do
          GitlabNet.any_instance.stub(:merge_request_urls).with(gl_repository, repo_path, wrongly_encoded_changes) do
            new_merge_request_urls
          end
          GitlabNet.any_instance.stub(broadcast_message: { "message" => broadcast_message })
        end

        it 'prints the broadcast message and create new merge request link' do
          assert_broadcast_message_printed(gitlab_post_receive)
          assert_new_mr_printed(gitlab_post_receive)

          gitlab_post_receive.exec
        end
      end

      context 'Sidekiq jobs' do
        it "pushes a Sidekiq job onto the queue" do
          expect(redis_client).to receive(:rpush).with(
            'resque:gitlab:queue:post_receive',
            %Q/{"class":"PostReceive","args":["#{gl_repository}","#{actor}",#{base64_changes.inspect}],"jid":"#{gitlab_post_receive.jid}","enqueued_at":#{enqueued_at.to_f}}/
          ).and_return(true)

          gitlab_post_receive.exec
        end

        context 'when gl_repository is nil' do
          let(:gl_repository) { nil }

          it "pushes a Sidekiq job with the repository path" do
            expect(redis_client).to receive(:rpush).with(
              'resque:gitlab:queue:post_receive',
              %Q/{"class":"PostReceive","args":["#{repo_path}","#{actor}",#{base64_changes.inspect}],"jid":"#{gitlab_post_receive.jid}","enqueued_at":#{enqueued_at.to_f}}/
            ).and_return(true)

            gitlab_post_receive.exec
          end
        end
      end

      context 'reference counter' do
        it 'decreases the reference counter for the project' do
          expect_any_instance_of(GitlabReferenceCounter).to receive(:decrease).and_return(true)

          gitlab_post_receive.exec
        end

        context "when the redis command succeeds" do
          before do
            allow(redis_client).to receive(:decr).and_return(0)
          end

          it "returns true" do
            expect(gitlab_post_receive.exec).to eq(true)
          end
        end

        context "when the redis command fails" do
          before do
            allow(redis_client).to receive(:decr).and_raise('Fail')
          end

          it "returns false" do
            expect(gitlab_post_receive.exec).to eq(false)
          end
        end
      end

      context 'post_receive notification' do
        it 'calls the api to notify the execution of the hook' do
          expect_any_instance_of(GitlabNet).to receive(:notify_post_receive).
            with(gl_repository, repo_path)

          gitlab_post_receive.exec
        end
      end

      context "when the redis command succeeds" do
        before do
          allow(redis_client).to receive(:rpush).and_return(true)
        end

        it "returns true" do
          expect(gitlab_post_receive.exec).to eq(true)
        end
      end

      context "when the redis command fails" do
        before do
          allow(redis_client).to receive(:rpush).and_raise('Fail')
        end

        it "returns false" do
          expect(gitlab_post_receive.exec).to eq(false)
        end
      end
    end

    context 'when the new post_receive API endpoint is available' do
      let(:response) { { 'reference_counter_decreased' => true } }

      it 'calls the api to notify the execution of the hook' do
        expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)

        expect(gitlab_post_receive.exec).to eq(true)
      end

      context 'merge request urls and broadcast messages' do
        let(:response) do
          {
            'reference_counter_decreased' => true,
            'merge_request_urls' => new_merge_request_urls,
            'broadcast_message' => broadcast_message
          }
        end

        it 'prints the merge request urls and broadcast message' do
          expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)
          assert_broadcast_message_printed(gitlab_post_receive)
          assert_new_mr_printed(gitlab_post_receive)

          expect(gitlab_post_receive.exec).to eq(true)
        end
      end
    end
  end

  private

  def assert_new_mr_printed(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "To create a merge request for new_branch, visit:"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "  http://localhost/dzaporozhets/gitlab-ci/merge_requests/new?merge_request%5Bsource_branch%5D=new_branch"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).ordered
  end

  def assert_existing_mr_printed(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "View merge request for feature_branch:"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "  http://localhost/dzaporozhets/gitlab-ci/merge_requests/1"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).ordered
  end

  def assert_broadcast_message_printed(gitlab_post_receive)
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
  end
end
