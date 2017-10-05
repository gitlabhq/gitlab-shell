require 'pathname'

class ObjectDirsHelper
  class << self
    def all_attributes
      {
        "GIT_ALTERNATE_OBJECT_DIRECTORIES" => absolute_alt_object_dirs,
        "GIT_ALTERNATE_OBJECT_DIRECTORIES_RELATIVE" => relative_alt_object_dirs,
        "GIT_OBJECT_DIRECTORY" => absolute_object_dir,
        "GIT_OBJECT_DIRECTORY_RELATIVE" => relative_object_dir
      }
    end

    def absolute_object_dir
      ENV['GIT_OBJECT_DIRECTORY']
    end

    def relative_object_dir
      relative_path(absolute_object_dir)
    end

    def absolute_alt_object_dirs
      ENV['GIT_ALTERNATE_OBJECT_DIRECTORIES'].to_s.split(File::PATH_SEPARATOR)
    end

    def relative_alt_object_dirs
      absolute_alt_object_dirs.map { |dir| relative_path(dir) }.compact
    end

    private

    def relative_path(absolute_path)
      return if absolute_path.nil?

      repo_dir = Dir.pwd
      Pathname.new(absolute_path).relative_path_from(Pathname.new(repo_dir)).to_s
    end
  end
end
