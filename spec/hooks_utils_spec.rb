require_relative 'spec_helper'
require_relative '../lib/hooks_utils.rb'

describe :get_push_options do
  context "when GIT_PUSH_OPTION_COUNT is not set" do
    HooksUtils.get_push_options.should == []
  end

  context "when one option is given" do
    ENV['GIT_PUSH_OPTION_COUNT'] = '1'
    ENV['GIT_PUSH_OPTION_0'] = 'aaa'
    HooksUtils.get_push_options.should == ['aaa']
  end

  context "when multiple options are given" do
    ENV['GIT_PUSH_OPTION_COUNT'] = '3'
    ENV['GIT_PUSH_OPTION_0'] = 'aaa'
    ENV['GIT_PUSH_OPTION_1'] = 'bbb'
    ENV['GIT_PUSH_OPTION_2'] = 'ccc'
    HooksUtils.get_push_options.should == ['aaa', 'bbb', 'ccc']
  end
end
