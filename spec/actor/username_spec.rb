require_relative '../spec_helper'
require_relative '../../lib/actor/username'

describe Actor::Username do
  let(:username) { 'testuser' }
  let(:api) { double(GitlabNet) }

  let(:discover_payload) { { 'username' => username } }
  let(:audit_usernames) { nil }

  before do
    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(subject).and_return(discover_payload)
  end

  describe '.from' do
    it 'returns an instance of Actor::Username' do
      expect(described_class.from("username-#{username}")).to be_a(Actor::Username)
    end

    it 'has an id == 1' do
      expect(described_class.from('username-1').id).to eq '1'
    end
  end

  describe '.identifier_prefix' do
    it "returns 'user'" do
      expect(described_class.identifier_prefix).to eql 'username'
    end
  end

  describe '.identifier_key' do
    it "returns 'username'" do
      expect(described_class.identifier_key).to eql 'username'
    end
  end

  subject { described_class.new(username, audit_usernames: audit_usernames) }

  describe '#username' do
    context 'without a valid user' do
      it "returns '@testuser'" do
        expect(subject.username).to eql "@#{username}"
      end
    end

    context 'without a valid user' do
      let(:discover_payload) { nil }

      it "returns 'Anonymous'" do
        expect(subject.username).to eql 'Anonymous'
      end
    end
  end

  describe '#identifier' do
    it "returns 'username-testuser'" do
      expect(subject.identifier).to eql 'username-testuser'
    end
  end

  describe '#log_username' do
    context 'when audit_usernames is true' do
      let(:audit_usernames) { true }

      it "returns '@testuser'" do
        expect(subject.log_username).to eql "@#{username}"
      end
    end

    context 'when audit_usernames is false' do
      let(:audit_usernames) { false }

      it "returns 'user with identifier username-testuser'" do
        expect(subject.log_username).to eql "user with identifier username-#{username}"
      end
    end
  end
end
