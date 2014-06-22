require_relative 'spec_helper'
require_relative '../lib/gitlab_logger'

describe :convert_log_level do
  subject { convert_log_level :extreme }

  it "converts invalid log level to Logger::INFO" do
    $stderr.should_receive(:puts).at_least(:once)
    should eq(Logger::INFO)
  end
end
