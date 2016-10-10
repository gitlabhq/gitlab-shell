require_relative 'spec_helper'
require_relative '../lib/gitlab_metrics'

describe GitlabMetrics do
  describe '::measure' do
    it 'returns the return value of the block' do
      val = described_class.measure('foo') { 10 }

      expect(val).to eq(10)
    end

    it 'write in a file metrics data' do
      result = nil
      expect(described_class.logger).to receive(:debug) do |&b|
        result = b.call
      end

      described_class.measure('foo') { 10 }

      expect(result).to match(/name=\"foo\" wall_time=\d+ cpu_time=\d+/)
    end

    it 'calls proper measure methods' do
      expect(described_class::System).to receive(:monotonic_time).twice.and_call_original
      expect(described_class::System).to receive(:cpu_time).twice.and_call_original

      described_class.measure('foo') { 10 }
    end
  end
end
