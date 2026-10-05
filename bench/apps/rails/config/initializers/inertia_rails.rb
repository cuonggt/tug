InertiaRails.configure do |config|
  # The asset version every app in the benchmark has.
  config.version = "1"

  # Inertia v3's client reads the first visit's page only from a
  # <script type="application/json"> element, not from the data-page attribute
  # the gem writes by default; `rails generate inertia:install` turns this on.
  config.use_script_element_for_initial_page = true

  # Share an empty `errors` on every page, as Inertia's protocol has it and
  # `inertia:install` sets up. Left unset, the gem warns on stderr at every
  # render that this will be its default.
  config.always_include_errors_hash = true
end
