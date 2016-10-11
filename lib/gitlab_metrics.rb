require 'logger'
require_relative 'gitlab_config'

module GitlabMetrics
  module System
    # THREAD_CPUTIME is not supported on OS X
    if Process.const_defined?(:CLOCK_THREAD_CPUTIME_ID)
      def self.cpu_time
        Process.
          clock_gettime(Process::CLOCK_THREAD_CPUTIME_ID, :millisecond)
      end
    else
      def self.cpu_time
        Process.
          clock_gettime(Process::CLOCK_PROCESS_CPUTIME_ID, :millisecond)
      end
    end

    # Returns the current monotonic clock time in a given precision.
    #
    # Returns the time as a Fixnum.
    def self.monotonic_time
      Process.clock_gettime(Process::CLOCK_MONOTONIC, :millisecond)
    end
  end

  def self.logger
    @logger ||= Logger.new(GitlabConfig.new.metrics_log_file)
  end

  # Measures the execution time of a block.
  #
  # Example:
  #
  #     GitlabMetrics.measure(:find_by_username_duration) do
  #       User.find_by_username(some_username)
  #     end
  #
  # name - The name of the field to store the execution time in.
  #
  # Returns the value yielded by the supplied block.
  def self.measure(name)
    start_real = System.monotonic_time
    start_cpu = System.cpu_time

    retval = yield

    real_time = System.monotonic_time - start_real
    cpu_time = System.cpu_time - start_cpu

    logger.debug { "name=#{name.inspect} wall_time=#{real_time.inspect} cpu_time=#{cpu_time.inspect}" }

    retval
  end
end
