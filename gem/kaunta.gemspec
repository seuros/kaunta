# frozen_string_literal: true

Gem::Specification.new do |spec|
  # Set by the release pipeline to build a platform gem carrying the native
  # binary; unset builds the stub that explains how to get one.
  platform_release = ENV["KAUNTA_PLATFORM"]

  spec.name = "kaunta"
  spec.version = ENV["KAUNTA_VERSION"] || "0.121.0"
  spec.authors = ["Abdelkader Boudih"]
  spec.email = ["oss@seuros.com"]

  spec.summary = "Self-hosted, privacy-first web analytics"
  spec.description = if platform_release
                       "Kaunta with a bundled native binary for this platform. " \
                       "Run `kaunta serve` to start the server, or `kaunta --help` for the CLI."
                     else
                       "Stub gem for Kaunta. Platform-specific releases bundle the native binary."
                     end
  spec.homepage = "https://github.com/seuros/kaunta"
  spec.license = "MIT"
  spec.required_ruby_version = ">= 3.0"

  spec.metadata["source_code_uri"] = spec.homepage
  spec.metadata["changelog_uri"] = "#{spec.homepage}/blob/master/CHANGELOG.md"
  spec.metadata["rubygems_mfa_required"] = "true"

  if platform_release
    spec.platform = Gem::Platform.new(platform_release)
    spec.files = %w[exe/kaunta libexec/kaunta README.md LICENSE]
  else
    spec.platform = Gem::Platform::RUBY
    spec.files = %w[exe/kaunta README.md LICENSE]
  end

  spec.bindir = "exe"
  spec.executables = ["kaunta"]
end
