require_relative 'spec_helper'
require_relative '../lib/hooks_utils.rb'

describe :get_push_options do
  context "when GIT_PUSH_OPTION_COUNT is not set" do
    it { expect(HooksUtils.get_push_options).to eq([]) }
  end

  context "when one option is given" do
    before do
      ENV['GIT_PUSH_OPTION_COUNT'] = '1'
      ENV['GIT_PUSH_OPTION_0'] = 'aaa'
    end

    it { expect(HooksUtils.get_push_options).to eq(['aaa']) }
  end

  context "when multiple options are given" do
    before do
      ENV['GIT_PUSH_OPTION_COUNT'] = '3'
      ENV['GIT_PUSH_OPTION_0'] = 'aaa'
      ENV['GIT_PUSH_OPTION_1'] = 'bbb'
      ENV['GIT_PUSH_OPTION_2'] = 'ccc'
    end

    it { expect(HooksUtils.get_push_options).to eq(['aaa', 'bbb', 'ccc']) }
  end
end
