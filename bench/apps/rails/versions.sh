#!/usr/bin/env bash
# Prints what the app runs on, one `Name version` a line: Ruby as the app has
# it once booted in production, +YJIT when Rails has turned YJIT on (the
# workers fork from that boot), and the gems as Gemfile.lock pins them.
# Run setup.sh first.
set -euo pipefail
cd "$(dirname "$0")"

# SECRET_KEY_BASE_DUMMY: Rails' own stand-in for a boot that needs no secret.
RAILS_ENV=production RAILS_LOG_LEVEL=warn SECRET_KEY_BASE_DUMMY=1 bin/rails runner '
  yjit = defined?(RubyVM::YJIT) && RubyVM::YJIT.enabled? ? " +YJIT" : ""
  puts "Ruby #{RUBY_VERSION}#{yjit}"
  { "Rails" => "rails", "inertia_rails" => "inertia_rails", "Puma" => "puma" }.each do |name, gem|
    puts "#{name} #{Gem.loaded_specs.fetch(gem).version}"
  end
'
