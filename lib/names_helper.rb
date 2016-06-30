module NamesHelper
  def extract_repo_name(path)
    repo_name = path.strip
    repo_name.gsub!(/\.git$/, "")
    repo_name.gsub!(/^\//, "")
    repo_name.split(File::SEPARATOR).last(2).join(File::SEPARATOR)
  end

  def extract_ref_name(ref)
    ref.gsub(/\Arefs\/(tags|heads)\//, '')
  end
end
