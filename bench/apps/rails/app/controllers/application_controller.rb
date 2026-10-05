class ApplicationController < ActionController::Base
  # Without the `allow_browser versions: :modern` that `rails new` writes
  # here: it reads each request's User-Agent, to turn away browsers too old
  # for a frontend's CSS and JavaScript, and the page has no frontend.
end
