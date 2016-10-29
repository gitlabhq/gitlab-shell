module NamesHelper
  def extract_ref_name(ref)
    ref.gsub(/\Arefs\/(tags|heads)\//, '')
  end
end
