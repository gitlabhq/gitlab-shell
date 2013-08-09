ROOT_PATH = File.expand_path(File.join(File.dirname(__FILE__), ".."))

if ENV['TRAVIS']
  require 'coveralls'
  Coveralls.wear!
end

require 'vcr'
require 'webmock'

VCR.configure do |c|
  c.cassette_library_dir = 'spec/vcr_cassettes'
  c.hook_into :webmock
  c.configure_rspec_metadata!
end

module ExitCodeMatchers
  extend RSpec::Matchers::DSL

  matcher :terminate do
    actual = nil

    match do |block|
      begin
        block.call
      rescue SystemExit => e
        actual = e.status
      end
      actual and actual == status_code
    end

    chain :with_code do |status_code|
      @status_code = status_code
    end

    failure_message_for_should do |block|
      "expected block to call exit(#{status_code}) but exit" +
        (actual.nil? ? " not called" : "(#{actual}) was called")
    end

    failure_message_for_should_not do |block|
      "expected block not to call exit(#{status_code})"
    end

    description do
      "expect block to call exit(#{status_code})"
    end

    def status_code
      @status_code ||= 0
    end
  end
end

RSpec.configure do |config|
  config.include ExitCodeMatchers
end