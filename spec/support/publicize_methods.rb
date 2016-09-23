# Temporarly convert all methods of an object to private to allow
# unit testing of private methods

class Class
  def publicize_methods
    private_methods = private_instance_methods
    class_eval { public *private_methods }
    # run the code with private methods apearing public
    yield
    class_eval { private *private_methods }
  end
end
