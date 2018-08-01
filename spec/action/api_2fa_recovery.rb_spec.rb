require_relative '../spec_helper'
require_relative '../../lib/action/api_2fa_recovery'

describe Action::API2FARecovery do
  let(:key_id) { '1' }
  let(:actor) { Actor::Key.new(key_id) }
  let(:username) { 'testuser' }
  let(:discover_payload) { { 'username' => username } }
  let(:api) { double(GitlabNet) }

  before do
    allow(GitlabNet).to receive(:new).and_return(api)
    allow(api).to receive(:discover).with(actor).and_return(discover_payload)
  end

  subject do
    described_class.new(actor)
  end

  describe '#execute' do
    context 'with an invalid repsonse' do
      it 'returns nil' do
        expect($stdin).to receive(:gets).and_return("meh\n")

        expect do
          expect(subject.execute(nil, nil)).to be_nil
        end.to output(/New recovery codes have \*not\* been generated/).to_stdout
      end
    end

    context 'with a negative response' do
      before do
        expect(subject).to receive(:continue?).and_return(false)
      end

      it 'returns nil' do
        expect do
          expect(subject.execute(nil, nil)).to be_nil
        end.to output(/New recovery codes have \*not\* been generated/).to_stdout
      end
    end


    context 'with an affirmative response' do
      let(:recovery_codes) { %w{ 8dfe0f433208f40b289904c6072e4a72 c33cee7fd0a73edb56e61b785e49af03 } }

      before do
        expect(subject).to receive(:continue?).and_return(true)
        expect(api).to receive(:two_factor_recovery_codes).with(subject).and_return(response)
      end

      context 'with a unsuccessful response' do
        let(:response) { { 'success' => false } }

        it 'puts error message to stdout' do
          expect do
            expect(subject.execute(nil, nil)).to be_falsey
          end.to output(/An error occurred while trying to generate new recovery codes/).to_stdout
        end
      end

      context 'with a successful response' do
        let(:response) { { 'success' => true, 'recovery_codes' => recovery_codes } }

        it 'puts information message including recovery codes to stdout' do
          expect do
            expect(subject.execute(nil, nil)).to be_truthy
          end.to output(Regexp.new(recovery_codes.join("\n"))).to_stdout
        end
      end
    end
  end
end
