require 'spec_helper'
require 'gitlab_lfs_authentication'

describe GitlabLfsAuthentication do
  let(:user) { { 'username' => 'dzaporozhets', 'lfs_token' => 'wsnys8Zm8Jn7zyhHTAAK' } }

  subject do
    GitlabLfsAuthentication.new(user, 'http://gitlab.dev/repo')
  end

  describe '#initialize' do
    it { subject.user.should == user }
    it { subject.repository_http_path.should == 'http://gitlab.dev/repo' }
  end

  describe '#authenticate!' do
    result = "{\"header\":{\"Authorization\":\"Basic ZHphcG9yb3poZXRzOndzbnlzOFptOEpuN3p5aEhUQUFL\"},\"href\":\"http://gitlab.dev/repo/info/lfs/\"}"

    it { subject.authenticate!.should == result }
  end
end
