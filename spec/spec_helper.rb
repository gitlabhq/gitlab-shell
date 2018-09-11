require 'rspec-parameterized'
require 'simplecov'
SimpleCov.start

require 'gitlab_init'

Dir[File.expand_path('support/**/*.rb', __dir__)].each { |f| require f }

RSpec.configure do |config|
  config.run_all_when_everything_filtered = true
  config.filter_run :focus

  config.before(:each) do
    stub_const('ROOT_PATH', File.expand_path('..', __dir__))
  end
end
