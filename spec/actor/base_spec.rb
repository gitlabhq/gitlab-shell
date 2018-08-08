require_relative '../spec_helper'
require_relative '../../lib/actor/base'

describe Actor::Base do
  describe '.identifier_key' do
    it 'raises a NotImplementedError exception' do
      expect do
        described_class.identifier_key
      end.to raise_error(NotImplementedError)
    end
  end

  describe '.identifier_prefix' do
    it 'raises a NotImplementedError exception' do
      expect do
        described_class.identifier_prefix
      end.to raise_error(NotImplementedError)
    end
  end

  describe '.id_regex' do
    it 'raises a NotImplementedError exception' do
      expect do
        described_class.id_regex
      end.to raise_error(NotImplementedError)
    end
  end
end
