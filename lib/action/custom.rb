require 'base64'

require_relative '../http_helper'

module Action
  class Custom
    include HTTPHelper

    class BaseError < StandardError; end
    class MissingPayloadError < BaseError; end
    class MissingAPIEndpointsError < BaseError; end
    class MissingDataError < BaseError; end
    class UnsuccessfulError < BaseError; end

    NO_MESSAGE_TEXT = 'No message'.freeze
    DEFAULT_HEADERS = { 'Content-Type' => CONTENT_TYPE_JSON }.freeze

    def initialize(gl_id, payload)
      @gl_id = gl_id
      @payload = payload
    end

    def execute
      validate!
      result = process_api_endpoints

      if result && HTTP_SUCCESS_CODES.include?(result.code)
        result
      else
        raise_unsuccessful!(result)
      end
    end

    private

    attr_reader :gl_id, :payload

    def process_api_endpoints
      output = ''
      resp = nil

      data_with_gl_id = data.merge('gl_id' => gl_id)

      api_endpoints.each do |endpoint|
        url = "#{base_url}#{endpoint}"
        json = { 'data' => data_with_gl_id, 'output' => output }

        resp = post(url, {}, headers: DEFAULT_HEADERS, options: { json: json })
        return resp unless HTTP_SUCCESS_CODES.include?(resp.code)

        begin
          body = JSON.parse(resp.body)
        rescue JSON::ParserError
          raise UnsuccessfulError, 'Response was not valid JSON'
        end

        print_flush(body['result'])

        # In the context of the git push sequence of events, it's necessary to read
        # stdin in order to capture output to pass onto subsequent commands
        output = read_stdin
      end

      resp
    end

    def base_url
      config.gitlab_url
    end

    def data
      @data ||= payload['data']
    end

    def api_endpoints
      data['api_endpoints']
    end

    def config
      @config ||= GitlabConfig.new
    end

    def api
      @api ||= GitlabNet.new
    end

    def read_stdin
      Base64.encode64($stdin.read)
    end

    def print_flush(str)
      return false unless str
      $stdout.print(Base64.decode64(str))
      $stdout.flush
    end

    def validate!
      validate_payload!
      validate_data!
      validate_api_endpoints!
    end

    def validate_payload!
      raise MissingPayloadError if !payload.is_a?(Hash) || payload.empty?
    end

    def validate_data!
      raise MissingDataError unless data.is_a?(Hash)
    end

    def validate_api_endpoints!
      raise MissingAPIEndpointsError if !api_endpoints.is_a?(Array) ||
                                        api_endpoints.empty?
    end

    def raise_unsuccessful!(result)
      message = begin
        body = JSON.parse(result.body)
        message = body['message']
        message = Base64.decode64(body['result']) if !message && body['result'] && !body['result'].empty?
        message ? message : NO_MESSAGE_TEXT
      rescue JSON::ParserError
        NO_MESSAGE_TEXT
      end

      raise UnsuccessfulError, "#{message} (#{result.code})"
    end
  end
end
