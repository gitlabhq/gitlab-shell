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
