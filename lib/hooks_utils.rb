module HooksUtils
  module_function

  # Gets an array of Git push options from the environment
  def get_push_options
    count = ENV['GIT_PUSH_OPTION_COUNT'].to_i
    result = []

    count.times do |i|
      result.push(ENV["GIT_PUSH_OPTION_#{i}"])
    end

    result
  end
end
