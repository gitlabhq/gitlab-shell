# frozen_string_literal: true

module ConsoleHelper
  LINE_PREFACE = '> GitLab:'

  def write_stderr(messages)
    format_for_stderr(messages).each do |message|
      $stderr.puts(message)
    end
  end

  def format_for_stderr(messages)
    Array(messages).each_with_object([]) do |message, all|
      all << "#{LINE_PREFACE} #{message}" unless message.empty?
    end
  end
end
