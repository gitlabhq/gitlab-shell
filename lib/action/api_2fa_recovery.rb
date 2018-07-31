require_relative '../action'
require_relative '../gitlab_logger'

module Action
  class API2FARecovery < Base
    def initialize(key)
      @key = key
    end

    def execute(_, _)
      recover
    end

    private

    attr_reader :key

    def continue?(question)
      puts "#{question} (yes/no)"
      STDOUT.flush # Make sure the question gets output before we wait for input
      response = STDIN.gets.chomp
      puts '' # Add a buffer in the output
      response == 'yes'
    end

    def recover
      continue = continue?(
        "Are you sure you want to generate new two-factor recovery codes?\n" \
        "Any existing recovery codes you saved will be invalidated."
      )

      unless continue
        puts 'New recovery codes have *not* been generated. Existing codes will remain valid.'
        return
      end

      resp = api.two_factor_recovery_codes(key.key_id)
      if resp['success']
        codes = resp['recovery_codes'].join("\n")
        $logger.info('API 2FA recovery success', user: key.log_username)
        puts "Your two-factor authentication recovery codes are:\n\n" \
            "#{codes}\n\n" \
            "During sign in, use one of the codes above when prompted for\n" \
            "your two-factor code. Then, visit your Profile Settings and add\n" \
            "a new device so you do not lose access to your account again."
        true
      else
        $logger.info('API 2FA recovery error', user: key.log_username)
        puts "An error occurred while trying to generate new recovery codes.\n" \
            "#{resp['message']}"
      end
    end
  end
end
