require_relative 'spec_helper'
require_relative '../lib/gitlab_metrics'

describe GitlabMetrics do
  describe '.measure' do
    before do
      $logger = double('logger').as_null_object
    end

    it 'returns the return value of the block' do
      val = described_class.measure('foo') { 10 }

      expect(val).to eq(10)
    end

    it 'writes the metrics data to a log file' do
      expect($logger).to receive(:debug).
        with('metrics', a_metrics_log_message('foo'))

      described_class.measure('foo') { 10 }
    end

    it 'calls proper measure methods' do
      expect(described_class::System).to receive(:monotonic_time).twice.and_call_original
      expect(described_class::System).to receive(:cpu_time).twice.and_call_original

      described_class.measure('foo') { 10 }
    end
  end
end

RSpec::Matchers.define :a_metrics_log_message do |x|
  match do |actual|
    [
      actual.fetch(:name) == x,
      actual.fetch(:wall_time).is_a?(Numeric),
      actual.fetch(:cpu_time).is_a?(Numeric),
    ].all?
  end
end
