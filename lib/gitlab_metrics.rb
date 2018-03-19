require_relative 'gitlab_config'
require_relative 'gitlab_logger'

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
      if defined?(Process::CLOCK_MONOTONIC)
        Process.clock_gettime(Process::CLOCK_MONOTONIC, :millisecond)
      else
        Process.clock_gettime(Process::CLOCK_REALTIME, :millisecond)
      end
    end
  end

  def self.logger
    $logger
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

    logger.debug('metrics', name: name, wall_time: real_time, cpu_time: cpu_time)

    retval
  end
end
