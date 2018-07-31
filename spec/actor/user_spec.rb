require_relative '../spec_helper'
require_relative '../../lib/actor/user'

describe Actor::User do
  let(:user_id) { '1' }
  let(:username) { 'testuser' }
  let(:audit_usernames) { nil }

  describe '.from' do
    it 'returns an instance of Actor::User' do
      expect(described_class.from('user-1')).to be_a(Actor::User)
    end

    it 'has an id == 1' do
      expect(described_class.from('user-1').id).to eq '1'
    end
  end

  describe '.identifier_prefix' do
    it "returns 'user'" do
      expect(described_class.identifier_prefix).to eql 'user'
    end
  end

  describe '.identifier_key' do
    it "returns 'user_id'" do
      expect(described_class.identifier_key).to eql 'user_id'
    end
  end

  subject { described_class.new(user_id, audit_usernames: audit_usernames) }

  describe '#username' do
    it "returns 'user-1'" do
      expect(subject.username).to eql 'user-1'
    end
  end

  describe '#identifier' do
    it "returns 'user-1'" do
      expect(subject.identifier).to eql 'user-1'
    end
  end

  describe '#log_username' do
    context 'when audit_usernames is true' do
      let(:audit_usernames) { true }

      it "returns 'user-1'" do
        expect(subject.log_username).to eql 'user-1'
      end
    end

    context 'when audit_usernames is false' do
      let(:audit_usernames) { false }

      it "returns 'user with identifier user-1'" do
        expect(subject.log_username).to eql 'user with identifier user-1'
      end
    end
  end
end
