require 'spec_helper'
require 'names_helper'

describe NamesHelper do
  include NamesHelper

  describe :extract_ref_name do
    it { expect(extract_ref_name('refs/heads/awesome-feature')).to eq('awesome-feature') }
    it { expect(extract_ref_name('refs/tags/v2.2.1')).to eq('v2.2.1') }
    it { expect(extract_ref_name('refs/tags/releases/v2.2.1')).to eq('releases/v2.2.1') }
  end
end
