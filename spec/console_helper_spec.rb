require_relative 'spec_helper'
require_relative '../lib/console_helper'

describe ConsoleHelper do
  using RSpec::Parameterized::TableSyntax

  class DummyClass
    include ConsoleHelper
  end

  subject { DummyClass.new }

  describe '#write_stderr' do
    where(:messages, :stderr_output) do
      'test'          | "> GitLab: test\n"
      %w{test1 test2} | "> GitLab: test1\n> GitLab: test2\n"
    end

    with_them do
      it 'puts to $stderr, prefaced with > GitLab:' do
        expect { subject.write_stderr(messages) }.to output(stderr_output).to_stderr
      end
    end
  end

  describe '#format_for_stderr' do
    where(:messages, :result) do
      'test'          | ['> GitLab: test']
      %w{test1 test2} | ['> GitLab: test1', '> GitLab: test2']
    end

    with_them do
      it 'returns message(s), prefaced with > GitLab:' do
          expect(subject.format_for_stderr(messages)).to eq(result)
      end
    end
  end
end
