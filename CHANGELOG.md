# Changelog

## [0.11.1](https://github.com/seuros/kaunta/compare/v0.11.0...v0.11.1) (2025-11-09)


### Bug Fixes

* **dashboard:** resolve chart rendering race condition ([#16](https://github.com/seuros/kaunta/issues/16)) ([de60e7b](https://github.com/seuros/kaunta/commit/de60e7b75afb602ed8205acabd2078c731d9a8e5))

## [0.11.0](https://github.com/seuros/kaunta/compare/v0.10.3...v0.11.0) (2025-11-09)


### Features

* add CSRF protection and rate limiting to login endpoint ([f260dd3](https://github.com/seuros/kaunta/commit/f260dd3d16460ebf7010490627045d5bb6b226cf))

## [0.10.3](https://github.com/seuros/kaunta/compare/v0.10.2...v0.10.3) (2025-11-09)


### Bug Fixes

* add checkout step to upload-release-assets job ([a586e32](https://github.com/seuros/kaunta/commit/a586e32c256782d6ad1aa4cf846ff9b28e90a090))

## [0.10.2](https://github.com/seuros/kaunta/compare/v0.10.1...v0.10.2) (2025-11-09)


### Bug Fixes

* tidy go module dependencies ([6bcd8e8](https://github.com/seuros/kaunta/commit/6bcd8e8265ab7d16347cca321ab647784f538000))

## [0.10.1](https://github.com/seuros/kaunta/compare/v0.10.0...v0.10.1) (2025-11-09)


### Bug Fixes

* update release workflow for self-upgrade support ([77e760c](https://github.com/seuros/kaunta/commit/77e760c4db0d0e1c0cf0653d2c69b1bcdd77a559))

## [0.10.0](https://github.com/seuros/kaunta/compare/v0.9.0...v0.10.0) (2025-11-09)


### Features

* add binary build and Docker push to release-please workflow ([20ec3c4](https://github.com/seuros/kaunta/commit/20ec3c4e543c623d949f1259fa6d020bde14e722))
* add test files and CLI improvements ([b9cc9af](https://github.com/seuros/kaunta/commit/b9cc9af8d05cbc533332c579c35316dfb013e47e))

## [0.9.0](https://github.com/seuros/kaunta/compare/v0.8.0...v0.9.0) (2025-11-08)


### Features

* add self-update functionality ([b54059a](https://github.com/seuros/kaunta/commit/b54059af7b1a7719df7ea724b30972df7d166113))
* add TOML config file support ([afb2e98](https://github.com/seuros/kaunta/commit/afb2e982838640badb41b7f779b21191fd0362cc))
* add username-based authentication system ([2a0c7f1](https://github.com/seuros/kaunta/commit/2a0c7f1a5d1821b6e4a3434eab10e84cfa6b1ceb))
* optimize tracker with rAF scroll batching, ResizeObserver, sendBeacon, AbortController cleanup, and test infrastructure ([d918084](https://github.com/seuros/kaunta/commit/d91808442fe65ee673d17f7bd0bd719136b4c5da))
* redesign home page and dashboard ([#9](https://github.com/seuros/kaunta/issues/9)) ([4ddc110](https://github.com/seuros/kaunta/commit/4ddc11062bf477e62d1bdb24e05af09934efb6da))
* redesign the login page ([#10](https://github.com/seuros/kaunta/issues/10)) ([0991de5](https://github.com/seuros/kaunta/commit/0991de552f41f386d313b651bdd212a4ef554891))

## [0.8.0](https://github.com/seuros/kaunta/compare/v0.7.1...v0.8.0) (2025-11-08)


### Features

* fix tracker initialization and dynamic ETag generation ([b5a9558](https://github.com/seuros/kaunta/commit/b5a95588599b8e8dd4727ccaa7e23cd0e9d98023))

## [0.7.1](https://github.com/seuros/kaunta/compare/v0.7.0...v0.7.1) (2025-11-08)


### Bug Fixes

* add debug ([23a3009](https://github.com/seuros/kaunta/commit/23a30091c261314e029635038071e7967ed6c71f))

## [0.7.0](https://github.com/seuros/kaunta/compare/v0.6.2...v0.7.0) (2025-11-07)


### Features

* fix defer/async script loading and add Playwright tests ([58a7a4f](https://github.com/seuros/kaunta/commit/58a7a4ff0604555f268b2b94f6291b41d0736b69))

## [0.6.2](https://github.com/seuros/kaunta/compare/v0.6.1...v0.6.2) (2025-11-07)


### Bug Fixes

* build ([d2b2792](https://github.com/seuros/kaunta/commit/d2b279276cd93721ada8fbfff3831fb46c2fc68f))
* extract index outside ([2f388c1](https://github.com/seuros/kaunta/commit/2f388c187360188693e8bd8d816e2b19e43602ef))

## [0.6.1](https://github.com/seuros/kaunta/compare/v0.6.0...v0.6.1) (2025-11-07)


### Bug Fixes

* bump version ([8053011](https://github.com/seuros/kaunta/commit/8053011c9fd290ef93806b11849983d9234b57c0))
* Show full website IDs in table format instead of truncated ([55cc9f8](https://github.com/seuros/kaunta/commit/55cc9f83d1cc09316c0581017afda5fb2d784fe3))
* use bun to compile js ([b933417](https://github.com/seuros/kaunta/commit/b933417b13d604087e7323a8fc0c936c9c7d5c7b))
