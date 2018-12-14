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
  let(:push_options) { [] }
  let(:gitlab_post_receive) { GitlabPostReceive.new(gl_repository, repo_path, actor, wrongly_encoded_changes, push_options) }
  let(:broadcast_message) { "test " * 10 + "message " * 10 }
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
    allow_any_instance_of(GitlabConfig).to receive(:repos_path).and_return(repository_path)
  end

  describe "#exec" do
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

      context 'when contains long url string at end' do
        let(:broadcast_message) { "test " * 10 + "message " * 10 + "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url" }

        it 'doesnt truncate url' do          
          expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)
          assert_broadcast_message_printed_keep_long_url_end(gitlab_post_receive)
          assert_new_mr_printed(gitlab_post_receive)

          expect(gitlab_post_receive.exec).to eq(true)
        end
      end

      context 'when contains long url string at start' do
        let(:broadcast_message) { "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url " + "test " * 10 + "message " * 11}

        it 'doesnt truncate url' do          
          expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)
          assert_broadcast_message_printed_keep_long_url_start(gitlab_post_receive)
          assert_new_mr_printed(gitlab_post_receive)

          expect(gitlab_post_receive.exec).to eq(true)
        end
      end

      context 'when contains long url string in middle' do
        let(:broadcast_message) { "test " * 11 + "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url " + "message " * 11}

        it 'doesnt truncate url' do          
          expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)
          assert_broadcast_message_printed_keep_long_url_middle(gitlab_post_receive)
          assert_new_mr_printed(gitlab_post_receive)

          expect(gitlab_post_receive.exec).to eq(true)
        end
      end

    end

    context 'when redirected message available' do
      let(:message) { "This is a redirected message" }
      let(:response) do
        {
          'reference_counter_decreased' => true,
          'redirected_message' => message
        }
      end

      it 'prints redirected message' do
        expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)
        assert_redirected_message_printed(gitlab_post_receive)
        expect(gitlab_post_receive.exec).to eq(true)
      end

      context 'when project created message is available' do
        let(:message) { "This is a created project message" }
        let(:response) do
          {
            'reference_counter_decreased' => true,
            'project_created_message' => message
          }
        end

        it 'prints project created message' do
          expect_any_instance_of(GitlabNet).to receive(:post_receive).and_return(response)

          assert_project_created_message_printed(gitlab_post_receive)

          expect(gitlab_post_receive.exec).to be true
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

  def assert_redirected_message_printed(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).with("This is a redirected message")
  end

  def assert_project_created_message_printed(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).with("This is a created project message")
  end

  def assert_broadcast_message_printed_keep_long_url_end(gitlab_post_receive)
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
    
    expect(gitlab_post_receive).to receive(:puts).with(
      "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "========================================================================"
    ).ordered
  end

  def assert_broadcast_message_printed_keep_long_url_start(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "========================================================================"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).ordered
    
    expect(gitlab_post_receive).to receive(:puts).with(
      "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "   test test test test test test test test test test message message"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "    message message message message message message message message"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "                                message"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "========================================================================"
    ).ordered
  end

  def assert_broadcast_message_printed_keep_long_url_middle(gitlab_post_receive)
    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "========================================================================"
    ).ordered
    expect(gitlab_post_receive).to receive(:puts).ordered
    
    expect(gitlab_post_receive).to receive(:puts).with(
      "         test test test test test test test test test test test"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "https://localhost:5000/test/a/really/long/url/that/is/in/the/broadcast/message/do-not-truncate-when-url"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "    message message message message message message message message"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).with(
      "                        message message message"
    ).ordered

    expect(gitlab_post_receive).to receive(:puts).ordered
    expect(gitlab_post_receive).to receive(:puts).with(
      "========================================================================"
    ).ordered
  end
end
