module NamesHelper
  def extract_repo_name(path, base)
    repo_name = path.strip
    repo_name.gsub!(base, "")
    repo_name.gsub!(/\.git$/, "")
    repo_name.gsub!(/^\//, "")
    repo_name
  end

  def extract_ref_name(ref)
    ref.gsub(/\Arefs\/(tags|heads)\//, '')
  end
end
