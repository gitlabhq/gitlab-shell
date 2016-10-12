require_relative 'spec_helper'
require_relative '../lib/gitlab_metrics'

describe GitlabMetrics do
  describe '.measure' do
    it 'returns the return value of the block' do
      val = described_class.measure('foo') { 10 }

      expect(val).to eq(10)
    end

    it 'writes the metrics data to a log file' do
      expect(described_class.logger).to receive(:debug).
        with(/metrics: name=\"foo\" wall_time=\d+ cpu_time=\d+/)

      described_class.measure('foo') { 10 }
    end

    it 'calls proper measure methods' do
      expect(described_class::System).to receive(:monotonic_time).twice.and_call_original
      expect(described_class::System).to receive(:cpu_time).twice.and_call_original

      described_class.measure('foo') { 10 }
    end
  end
end
