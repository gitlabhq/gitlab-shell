require_relative '../spec_helper'
require_relative '../../lib/action/base'

describe Action::Base do
  describe '.create_from_json' do
    it 'raises a NotImplementedError exeption' do
      expect do
        described_class.create_from_json('nomatter')
      end.to raise_error(NotImplementedError)
    end
  end
end
