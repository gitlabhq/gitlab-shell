require_relative 'spec_helper'
require_relative '../lib/object_dirs_helper'

describe ObjectDirsHelper do
  before do
    allow(Dir).to receive(:pwd).and_return('/home/git/repositories/foo/bar.git')
  end

  describe '.all_attributes' do
    it do
      expect(described_class.all_attributes.keys).to include(*%w[
                                                        GIT_OBJECT_DIRECTORY
                                                        GIT_OBJECT_DIRECTORY_RELATIVE
                                                        GIT_ALTERNATE_OBJECT_DIRECTORIES
                                                        GIT_ALTERNATE_OBJECT_DIRECTORIES_RELATIVE
                                                             ])
    end
  end

  describe '.absolute_object_dir' do
    subject { described_class.absolute_object_dir }

    context 'when GIT_OBJECT_DIRECTORY is set' do
      let(:dir) { '/home/git/repositories/foo/bar.git/./objects' }

      before do
        allow(ENV).to receive(:[]).with('GIT_OBJECT_DIRECTORY').and_return(dir)
      end

      it { expect(subject).to eq(dir) }
    end

    context 'when GIT_OBJECT_DIRECTORY is not set' do
      it { expect(subject).to be_nil }
    end
  end

  describe '.absolute_alt_object_dirs' do
    subject { described_class.absolute_alt_object_dirs }

    context 'when GIT_ALTERNATE_OBJECT_DIRECTORIES is set' do
      let(:dirs) { [
        '/home/git/repositories/foo/bar.git/./incoming-UKU6Gl',
        '/home/git/repositories/foo/bar.git/./incoming-AcU7Qr'
      ] }

      before do
        allow(ENV).to receive(:[]).with('GIT_ALTERNATE_OBJECT_DIRECTORIES').and_return(dirs.join(File::PATH_SEPARATOR))
      end

      it { expect(subject).to eq(dirs) }
    end

    context 'when GIT_ALTERNATE_OBJECT_DIRECTORIES is not set' do
      it { expect(subject).to eq([]) }
    end
  end

  describe '.relative_alt_object_dirs' do
    subject { described_class.relative_alt_object_dirs }

    context 'when GIT_ALTERNATE_OBJECT_DIRECTORIES is set' do
      let(:dirs) { [
        '/home/git/repositories/foo/bar.git/./objects/incoming-UKU6Gl',
        '/home/git/repositories/foo/bar.git/./objects/incoming-AcU7Qr'
      ] }

      before do
        allow(ENV).to receive(:[]).with('GIT_ALTERNATE_OBJECT_DIRECTORIES').and_return(dirs.join(File::PATH_SEPARATOR))
      end

      it { expect(subject).to eq(['objects/incoming-UKU6Gl', 'objects/incoming-AcU7Qr']) }
    end

    context 'when GIT_ALTERNATE_OBJECT_DIRECTORIES is not set' do
      it { expect(subject).to eq([]) }
    end
  end

  describe '.relative_object_dir' do
    subject { described_class.relative_object_dir }

    context 'when GIT_OBJECT_DIRECTORY is set' do
      before do
        allow(ENV).to receive(:[]).with('GIT_OBJECT_DIRECTORY').and_return('/home/git/repositories/foo/bar.git/./objects')
      end

      it { expect(subject).to eq('objects') }
    end

    context 'when GIT_OBJECT_DIRECTORY is not set' do
      it { expect(subject).to be_nil }
    end
  end
end
