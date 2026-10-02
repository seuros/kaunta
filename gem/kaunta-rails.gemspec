# frozen_string_literal: true

require_relative "lib/kaunta/rails/version"

Gem::Specification.new do |spec|
  spec.name = "kaunta-rails"
  spec.version = Kaunta::Rails::VERSION
  spec.authors = ["Abdelkader Boudih"]
  spec.email = ["oss@seuros.com"]

  spec.summary = "Send events to Kaunta from Rails"
  spec.description = "Server-side tracking client for Kaunta, the self-hosted " \
                     "privacy-first analytics server. Bundles no native binary; " \
                     "install the kaunta gem to run the server itself."
  spec.homepage = "https://github.com/seuros/kaunta"
  spec.license = "MIT"
  spec.required_ruby_version = ">= 3.2"

  spec.metadata["source_code_uri"] = spec.homepage
  spec.metadata["changelog_uri"] = "#{spec.homepage}/blob/master/CHANGELOG.md"
  spec.metadata["rubygems_mfa_required"] = "true"

  # Both gems are built from this directory, so list files explicitly:
  # kaunta ships exe/ and libexec/, this one ships lib/ only.
  spec.files = Dir["lib/**/*.rb"] + %w[README-rails.md LICENSE]
  spec.require_paths = ["lib"]
end
