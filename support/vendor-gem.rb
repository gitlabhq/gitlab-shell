#!/usr/bin/env ruby
require 'fileutils'
require 'open3'

VENDOR_PREFIX = 'lib/vendor'

def main(gem, version, vendor_dir)
  if !vendor_dir.start_with?(VENDOR_PREFIX)
    abort "Invalid vendor_dir: must start with #{VENDOR_PREFIX}: #{vendor_dir.inspect}"
  end

  FileUtils.rm_rf(vendor_dir)

  # Use clear-sources to force https://rubygems.org as the source
  if !system(*%W[gem fetch #{gem} -v #{version} --clear-sources -s https://rubygems.org])
    abort "Failed to fetch gem"
  end

  FileUtils.mkdir_p(vendor_dir)

  # A .gem file is a tar file containing a data.tar.gz file.
  statuses = Open3.pipeline(
    %W[tar -xOf - data.tar.gz],
    %W[tar -zxf -],
    in: "#{gem}-#{version}.gem",
    chdir: vendor_dir
  )

  if !statuses.all?(&:success?)
    abort "Failed to extract gem"
  end
end

if ARGV.count != 3
  abort "Usage: #{$PROGNAME} GEM VERSION VENDOR_DIR"
end

main(*ARGV)
