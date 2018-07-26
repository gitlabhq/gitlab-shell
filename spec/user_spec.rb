require_relative 'spec_helper'
require_relative '../lib/user'

describe User, vcr: true do
  let(:key_id) { 'key-1' }
  let(:username) { 'testuser' }
  let(:api) { double(GitlabNet) }

  let(:discover_payload) { { 'username' => username } }
  let(:audit_usernames) { nil }

  before do
    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(key_id).and_return(discover_payload)
  end

  subject { described_class.new(key_id, audit_usernames: audit_usernames) }

  describe '#username' do
    context 'with a valid user' do
      it "returns '@testuser'" do
        expect(subject.username).to eql '@testuser'
      end
    end

    context 'without a valid user' do
      let(:discover_payload) { nil }

      it "returns 'Anonymous'" do
        expect(subject.username).to eql 'Anonymous'
      end
    end
  end

  describe '#log_username' do
    context 'when audit_usernames is true' do
      let(:audit_usernames) { true }

      it "returns 'testuser'" do
        expect(subject.log_username).to eql '@testuser'
      end
    end

    context 'when audit_usernames is false' do
      let(:audit_usernames) { false }

      it "returns 'user with key key-1'" do
        expect(subject.log_username).to eql 'user with key key-1'
      end
    end
  end
end
