require_relative '../spec_helper'
require_relative '../../lib/actor/key'

describe Actor::Key do
  let(:key_id) { '1' }
  let(:username) { 'testuser' }
  let(:api) { double(GitlabNet) }

  let(:discover_payload) { { 'username' => username } }
  let(:audit_usernames) { nil }

  before do
    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(subject).and_return(discover_payload)
  end

  describe '.from' do
    it 'returns an instance of Actor::Key' do
      expect(described_class.from('key-1')).to be_a(Actor::Key)
    end

    it 'has a key_id == 1' do
      expect(described_class.from('key-1').key_id).to eq '1'
    end
  end

  describe '.identifier_prefix' do
    it "returns 'key'" do
      expect(described_class.identifier_prefix).to eql 'key'
    end
  end

  describe '.identifier_key' do
    it "returns 'key_id'" do
      expect(described_class.identifier_key).to eql 'key_id'
    end
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

  describe '#identifier' do
    it "returns 'key-1'" do
      expect(subject.identifier).to eql 'key-1'
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

      it "returns 'key with identifier key-1'" do
        expect(subject.log_username).to eql 'key with identifier key-1'
      end
    end
  end
end
