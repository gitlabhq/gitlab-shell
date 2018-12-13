require_relative 'spec_helper'
require_relative '../lib/gitlab_logger'
require 'securerandom'

describe :convert_log_level do
  subject { convert_log_level :extreme }

  it "converts invalid log level to Logger::INFO" do
    expect($stderr).to receive(:puts).at_least(:once)
    is_expected.to eq(Logger::INFO)
  end
end

describe GitlabLogger do
  subject { described_class.new(level, '/dev/null', format) }
  let(:format) { 'text' }
  let(:output) { StringIO.new }
  let(:level) { Logger::INFO }
  let(:time) { Time.at(123_456_789).utc } # '1973-11-29T21:33:09+00:00'
  let(:pid) { 1234 }

  before do
    allow(subject).to receive(:log_file).and_return(output)
    allow(subject).to receive(:time_now).and_return(time)
    allow(subject).to receive(:pid).and_return(pid)
  end

  def first_line
    output.string.lines.first.chomp
  end

  describe 'field sorting' do
    it 'sorts fields, except time, level, msg' do
      # Intentionally put 'foo' before 'baz' to see the effect of sorting
      subject.info('hello world', foo: 'bar', baz: 'qux')

      expect(first_line).to eq('time="1973-11-29T21:33:09+00:00" level=info msg="hello world" baz=qux foo=bar pid=1234')
    end
  end

  describe '#error' do
    context 'when the log level is too high' do
      let(:level) { Logger::FATAL }

      it 'does nothing' do
        subject.info('hello world')

        expect(output.string).to eq('')
      end
    end

    it 'logs data' do
      subject.error('hello world', foo: 'bar')

      expect(first_line).to eq('time="1973-11-29T21:33:09+00:00" level=error msg="hello world" foo=bar pid=1234')
    end
  end

  describe '#info' do
    context 'when the log level is too high' do
      let(:level) { Logger::ERROR }

      it 'does nothing' do
        subject.info('hello world')

        expect(output.string).to eq('')
      end
    end

    it 'logs data' do
      subject.info('hello world', foo: 'bar')

      expect(first_line).to eq('time="1973-11-29T21:33:09+00:00" level=info msg="hello world" foo=bar pid=1234')
    end
  end

  describe '#warn' do
    context 'when the log level is too high' do
      let(:level) { Logger::ERROR }

      it 'does nothing' do
        subject.warn('hello world')

        expect(output.string).to eq('')
      end
    end

    it 'logs data' do
      subject.warn('hello world', foo: 'bar')

      expect(first_line).to eq('time="1973-11-29T21:33:09+00:00" level=warn msg="hello world" foo=bar pid=1234')
    end
  end

  describe '#debug' do
    it 'does nothing' do
      subject.debug('hello world')

      expect(output.string).to eq('')
    end

    context 'when the log level is low enough' do
      let(:level) { Logger::DEBUG }

      it 'logs data' do
        subject.debug('hello world', foo: 'bar')

        expect(first_line).to eq('time="1973-11-29T21:33:09+00:00" level=debug msg="hello world" foo=bar pid=1234')
      end
    end
  end

  describe 'json logging' do
    let(:format) { 'json' }

    it 'writes valid JSON data' do
      subject.info('hello world', foo: 'bar')

      expect(JSON.parse(first_line)).to eq(
        'foo' => 'bar',
        'level' => 'info',
        'msg' => 'hello world',
        'pid' => 1234,
        'time' => '1973-11-29T21:33:09+00:00'
      )
    end

    it 'handles non-UTF8 string values' do
      subject.info("hello\x80world")

      expect(JSON.parse(first_line)).to include('msg' => '"hello\x80world"')
    end
  end

  describe 'log flushing' do
    it 'logs get written even when calling Kernel.exec' do
      msg = SecureRandom.hex(12)
      test_logger_status = system('bin/test-logger', msg)
      expect(test_logger_status).to eq(true)

      grep_status = system('grep', '-q', '-e', msg, GitlabConfig.new.log_file)
      expect(grep_status).to eq(true)
    end
  end
end
