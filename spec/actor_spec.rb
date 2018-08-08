require_relative 'spec_helper'
require_relative '../lib/actor'

describe Actor do
  describe '.new_from' do
    context 'for an unsupported Actor type' do
      it 'raises a NotImplementedError exception' do
        expect do
          described_class.new_from('unsupported-1')
        end.to raise_error(Actor::UnsupportedActorError)
      end
    end

    context 'for a supported Actor type' do
      context 'of Key' do
        it 'returns an instance of Key' do
          expect(described_class.new_from('key-1')).to be_a(Actor::Key)
        end
      end

      context 'of User' do
        it 'returns an instance of User' do
          expect(described_class.new_from('user-1')).to be_a(Actor::User)
        end
      end

      context 'of Username' do
        it 'returns an instance of Username' do
          expect(described_class.new_from('username-john1')).to be_a(Actor::Username)
        end
      end
    end
  end
end
