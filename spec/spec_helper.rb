ROOT_PATH = File.expand_path(File.join(File.dirname(__FILE__), ".."))

if ENV['TRAVIS']
  require 'coveralls'
  Coveralls.wear!
end
