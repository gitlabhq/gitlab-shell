require_relative '../spec_helper'
require_relative '../../lib/action/custom'

describe Action::Custom do
  let(:repo_name) { 'gitlab-ci.git' }
  let(:gl_id) { 'key-1' }
  let(:secret) { "0a3938d9d95d807e94d937af3a4fbbea" }
  let(:base_url) { 'http://localhost:3000' }

  subject { described_class.new(gl_id, payload) }

  describe '#execute' do
    context 'with an empty payload' do
      let(:payload) { {} }

      it 'raises a MissingPayloadError exception' do
        expect { subject.execute }.to raise_error(Action::Custom::MissingPayloadError)
      end
    end

    context 'with api_endpoints defined' do
      before do
        allow(subject).to receive(:base_url).and_return(base_url)
        allow(subject).to receive(:secret_token).and_return(secret)
        allow($stdin).to receive(:read).and_return('')
      end

      context 'that are valid' do
        where(:primary_repo_data) do
          [
            [ 'http://localhost:3001/user1/repo1.git' ],
            [{ 'http' => 'http://localhost:3001/user1/repo1.git' }],
            [{ 'http' => 'http://localhost:3001/user1/repo1.git', 'ssh' => 'ssh://user@localhost:3002/user1/repo1.git' }]
          ]
        end

        with_them do
          let(:payload) do
            {
              'action' => 'geo_proxy_to_primary',
              'data' => {
                'api_endpoints' => %w{/api/v4/fake/info_refs /api/v4/fake/push},
                'gl_username' => 'user1',
                'primary_repo' => primary_repo_data
              }
            }
          end

          context 'and responds correctly' do
            it 'prints a Base64 encoded result to $stdout' do
              VCR.use_cassette("custom-action-ok") do
                expect($stdout).to receive(:print).with('info_refs-result').ordered
                expect($stdout).to receive(:print).with('push-result').ordered
                subject.execute
              end
            end

            context 'with results printed to $stdout' do
              before do
                allow($stdout).to receive(:print).with('info_refs-result')
                allow($stdout).to receive(:print).with('push-result')
              end

              it 'prints a message to $stderr' do
                VCR.use_cassette("custom-action-ok-with-message") do
                  expect { subject.execute }.to output(/NOTE: Message here/).to_stderr
                end
              end

              it 'returns an instance of Net::HTTPCreated' do
                VCR.use_cassette("custom-action-ok") do
                  expect(subject.execute ).to be_instance_of(Net::HTTPCreated)
                end
              end
            end
          end

          context 'but responds incorrectly' do
            it 'raises an UnsuccessfulError exception' do
              VCR.use_cassette("custom-action-ok-not-json") do
                expect {
                  subject.execute
                }.to raise_error(Action::Custom::UnsuccessfulError, 'Response was not valid JSON')
              end
            end
          end
        end
      end

      context 'that are invalid' do
        context 'where api_endpoints gl_id is missing' do
          let(:payload) do
            {
              'action' => 'geo_proxy_to_primary',
              'data' => {
                'gl_username' => 'user1',
                'primary_repo' => 'http://localhost:3001/user1/repo1.git'
              }
            }
          end

          it 'raises a MissingAPIEndpointsError exception' do
            expect { subject.execute }.to raise_error(Action::Custom::MissingAPIEndpointsError)
          end
        end

        context 'where api_endpoints are empty' do
          let(:payload) do
            {
              'action' => 'geo_proxy_to_primary',
              'data' => {
                'api_endpoints' => [],
                'gl_username' => 'user1',
                'primary_repo' => 'http://localhost:3001/user1/repo1.git'
              }
            }
          end

          it 'raises a MissingAPIEndpointsError exception' do
            expect { subject.execute }.to raise_error(Action::Custom::MissingAPIEndpointsError)
          end
        end

        context 'where data gl_id is missing' do
          let(:payload) { { 'api_endpoints' => %w{/api/v4/fake/info_refs /api/v4/fake/push} } }

          it 'raises a MissingDataError exception' do
            expect { subject.execute }.to raise_error(Action::Custom::MissingDataError)
          end
        end

        context 'where API endpoints are bad' do
          let(:payload) do
            {
              'action' => 'geo_proxy_to_primary',
              'data' => {
                'api_endpoints' => %w{/api/v4/fake/info_refs_bad /api/v4/fake/push_bad},
                'gl_username' => 'user1',
                'primary_repo' => 'http://localhost:3001/user1/repo1.git'
              }
            }
          end

          context 'and response is JSON' do
            it 'raises an UnsuccessfulError exception' do
              VCR.use_cassette("custom-action-not-ok-json") do
                expect {
                  subject.execute
                }.to raise_error(Action::Custom::UnsuccessfulError, 'You cannot perform write operations on a read-only instance (403)')
              end
            end
          end

          context 'and response is not JSON' do
            it 'raises an UnsuccessfulError exception' do
              VCR.use_cassette("custom-action-not-ok-not-json") do
                expect {
                  subject.execute
                }.to raise_error(Action::Custom::UnsuccessfulError, 'No message (403)')
              end
            end
          end
        end
      end
    end
  end
end
