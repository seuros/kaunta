# Changelog

## [0.17.0](https://github.com/seuros/kaunta/compare/v0.16.0...v0.17.0) (2025-11-12)


### Features

* add real-time WebSocket updates and fix dashboard filters ([413e0ed](https://github.com/seuros/kaunta/commit/413e0ede747f339c3320c078dd0a3bd564e8b34b))
* migrate analytics queries to PostgreSQL functions ([8fc01cd](https://github.com/seuros/kaunta/commit/8fc01cd1a88d257d62d80cfb612cd1169b2d043b))

## [0.16.0](https://github.com/seuros/kaunta/compare/v0.15.4...v0.16.0) (2025-11-11)


### Features

* improve Docker workflow and fix dashboard issues ([a84647b](https://github.com/seuros/kaunta/commit/a84647b1b46bc36631a326953c89925f62f75a8d))


### Bug Fixes

* improve Docker healthcheck to test service health ([98a327e](https://github.com/seuros/kaunta/commit/98a327e0f1f419c04c7c1e414141abcb77d941a5))
* render empty chart and fix null array responses ([a7a087b](https://github.com/seuros/kaunta/commit/a7a087ba5dc4618a7d822e69df3af231d78e369c))

## [0.15.4](https://github.com/seuros/kaunta/compare/v0.15.3...v0.15.4) (2025-11-11)


### Bug Fixes

* introduce structured logging ([df99b17](https://github.com/seuros/kaunta/commit/df99b175e17a8428f6dd6df4c402829d13133120))

## [0.15.3](https://github.com/seuros/kaunta/compare/v0.15.2...v0.15.3) (2025-11-10)


### Bug Fixes

* fix release ([10b0397](https://github.com/seuros/kaunta/commit/10b03975e9cd544f3d1d53cc444253d745b2b4cd))

## [0.15.2](https://github.com/seuros/kaunta/compare/v0.15.1...v0.15.2) (2025-11-10)


### Bug Fixes

* test ci ([25c5067](https://github.com/seuros/kaunta/commit/25c5067fb2c09856b5fcd19659ff47b1a29aef9c))

## [0.15.1](https://github.com/seuros/kaunta/compare/v0.15.0...v0.15.1) (2025-11-10)


### Bug Fixes

* init release ([cd05be2](https://github.com/seuros/kaunta/commit/cd05be2a12ba5f0f69ac254414f70a098a957d4a))

## [0.15.0](https://github.com/seuros/kaunta/compare/v0.14.9...v0.15.0) (2025-11-10)


### Features

* add binary build and Docker push to release-please workflow ([20ec3c4](https://github.com/seuros/kaunta/commit/20ec3c4e543c623d949f1259fa6d020bde14e722))
* Add comprehensive CLI for website and analytics management ([d8239ce](https://github.com/seuros/kaunta/commit/d8239ceb2d92744464134eb3d0aa6888cdeed230))
* add CSRF protection and rate limiting to login endpoint ([f260dd3](https://github.com/seuros/kaunta/commit/f260dd3d16460ebf7010490627045d5bb6b226cf))
* add self-update functionality ([b54059a](https://github.com/seuros/kaunta/commit/b54059af7b1a7719df7ea724b30972df7d166113))
* add test files and CLI improvements ([b9cc9af](https://github.com/seuros/kaunta/commit/b9cc9af8d05cbc533332c579c35316dfb013e47e))
* add TOML config file support ([afb2e98](https://github.com/seuros/kaunta/commit/afb2e982838640badb41b7f779b21191fd0362cc))
* add username-based authentication system ([2a0c7f1](https://github.com/seuros/kaunta/commit/2a0c7f1a5d1821b6e4a3434eab10e84cfa6b1ceb))
* **auth:** add redirect middleware and fix CSRF configuration ([#20](https://github.com/seuros/kaunta/issues/20)) ([9ff5df3](https://github.com/seuros/kaunta/commit/9ff5df3017ce592792e3ebefa07ab1be5c2080e5))
* cleaner trusted origin ([962d225](https://github.com/seuros/kaunta/commit/962d225dac70f24d13a5304e71919559991704c5))
* enable prefork mode for bare metal, disable for Docker ([92d97e7](https://github.com/seuros/kaunta/commit/92d97e7a9e5568dba9d6e514d7fa57e86e7328d5))
* fix defer/async script loading and add Playwright tests ([58a7a4f](https://github.com/seuros/kaunta/commit/58a7a4ff0604555f268b2b94f6291b41d0736b69))
* fix tracker initialization and dynamic ETag generation ([b5a9558](https://github.com/seuros/kaunta/commit/b5a95588599b8e8dd4727ccaa7e23cd0e9d98023))
* optimize tracker with rAF scroll batching, ResizeObserver, sendBeacon, AbortController cleanup, and test infrastructure ([d918084](https://github.com/seuros/kaunta/commit/d91808442fe65ee673d17f7bd0bd719136b4c5da))
* redesign home page and dashboard ([#9](https://github.com/seuros/kaunta/issues/9)) ([4ddc110](https://github.com/seuros/kaunta/commit/4ddc11062bf477e62d1bdb24e05af09934efb6da))
* redesign the login page ([#10](https://github.com/seuros/kaunta/issues/10)) ([0991de5](https://github.com/seuros/kaunta/commit/0991de552f41f386d313b651bdd212a4ef554891))
* upgrade Fiber from v2 to v3 ([d901166](https://github.com/seuros/kaunta/commit/d90116665da61255586fb8ffef1d705042a33330))


### Bug Fixes

* add checkout step to upload-release-assets job ([a586e32](https://github.com/seuros/kaunta/commit/a586e32c256782d6ad1aa4cf846ff9b28e90a090))
* add credentials include to login fetch ([cd45a8b](https://github.com/seuros/kaunta/commit/cd45a8bcc0eb8592ca8ce1791b6aca4beb09eb4b))
* add debug ([23a3009](https://github.com/seuros/kaunta/commit/23a30091c261314e029635038071e7967ed6c71f))
* add POST to CORS allowed methods for tracking API ([3825051](https://github.com/seuros/kaunta/commit/3825051635d34d145465274c2e3c4dc196a34730))
* build ([d2b2792](https://github.com/seuros/kaunta/commit/d2b279276cd93721ada8fbfff3831fb46c2fc68f))
* bump version ([8053011](https://github.com/seuros/kaunta/commit/8053011c9fd290ef93806b11849983d9234b57c0))
* Critical production bugs and linter compliance ([1528ff5](https://github.com/seuros/kaunta/commit/1528ff5ffb056c8231a90bb945af5461dfaf1932))
* CSRF KeyLookup format for Fiber v2.52.9 compatibility ([d7406be](https://github.com/seuros/kaunta/commit/d7406be0792b43a01260519b9d2ea586a2262a3c))
* **dashboard:** resolve chart rendering race condition ([#16](https://github.com/seuros/kaunta/issues/16)) ([de60e7b](https://github.com/seuros/kaunta/commit/de60e7b75afb602ed8205acabd2078c731d9a8e5))
* enable CORS credentials for cookie auth ([88b7f44](https://github.com/seuros/kaunta/commit/88b7f446a8632d4d1a0ae6837b4f963985aea22c))
* extract index outside ([2f388c1](https://github.com/seuros/kaunta/commit/2f388c187360188693e8bd8d816e2b19e43602ef))
* include credentials in login fetch request for CSRF validation ([d51c56f](https://github.com/seuros/kaunta/commit/d51c56f45476dbd02b36d29fdd277cc05099a488))
* read CSRF token from cookie instead of server injection ([642cf05](https://github.com/seuros/kaunta/commit/642cf05b709ce25a2159b401f19a835e9497a686))
* Remove broken tests.yml and add linting to test.yml ([7f2f1af](https://github.com/seuros/kaunta/commit/7f2f1aff106544d8ff4b523dd883e0d389c0aab7))
* remove redundant CSRF endpoint and consolidate Fiber config ([a0ac37e](https://github.com/seuros/kaunta/commit/a0ac37ef2446cf8ad9556c627a6521e40a1f7b6a))
* resolve tracking endpoint failures - CHAR(2) country column constraint ([be074b9](https://github.com/seuros/kaunta/commit/be074b91a06822358fe13540b91dd172f8a4fe57))
* safe type assertion for CSRF token extraction ([684b077](https://github.com/seuros/kaunta/commit/684b0770d0e347546feb7483ff3e6c4afdeea145))
* Show full website IDs in table format instead of truncated ([55cc9f8](https://github.com/seuros/kaunta/commit/55cc9f83d1cc09316c0581017afda5fb2d784fe3))
* tidy go module dependencies ([6bcd8e8](https://github.com/seuros/kaunta/commit/6bcd8e8265ab7d16347cca321ab647784f538000))
* update GeoIP database URL and clean up version API response ([79ae811](https://github.com/seuros/kaunta/commit/79ae811bb3e6bc9b1150858d086d3ae34aed7545))
* update release workflow for self-upgrade support ([77e760c](https://github.com/seuros/kaunta/commit/77e760c4db0d0e1c0cf0653d2c69b1bcdd77a559))
* use AllowOriginsFunc for CORS credentials ([be62fc7](https://github.com/seuros/kaunta/commit/be62fc72970727691df51483e34d54001e8e2488))
* use bun to compile js ([b933417](https://github.com/seuros/kaunta/commit/b933417b13d604087e7323a8fc0c936c9c7d5c7b))
* use csrf.TokenFromContext in /api/auth/csrf endpoint ([4179df6](https://github.com/seuros/kaunta/commit/4179df66c0e6fdbbfcdacf7ebcbfe8ee269d55b6))
* use csrf.TokenFromContext() to properly extract CSRF token ([fcbb643](https://github.com/seuros/kaunta/commit/fcbb643972700718baccb957a07cf602b649a2c0))
* use SameSite=None for CSRF cookie ([e735470](https://github.com/seuros/kaunta/commit/e735470340e3e5b0edcb19069c801865eb7deb56))
* use TRUSTED_ORIGINS env var for CSRF with proxy ([c0eb17e](https://github.com/seuros/kaunta/commit/c0eb17e04287ad848bbd200de451a0a55d799c24))

## [0.14.9](https://github.com/seuros/kaunta/compare/v0.14.8...v0.14.9) (2025-11-10)


### Bug Fixes

* use TRUSTED_ORIGINS env var for CSRF with proxy ([c0eb17e](https://github.com/seuros/kaunta/commit/c0eb17e04287ad848bbd200de451a0a55d799c24))

## [0.14.8](https://github.com/seuros/kaunta/compare/v0.14.7...v0.14.8) (2025-11-10)


### Bug Fixes

* use SameSite=None for CSRF cookie ([e735470](https://github.com/seuros/kaunta/commit/e735470340e3e5b0edcb19069c801865eb7deb56))

## [0.14.7](https://github.com/seuros/kaunta/compare/v0.14.6...v0.14.7) (2025-11-10)


### Bug Fixes

* use AllowOriginsFunc for CORS credentials ([be62fc7](https://github.com/seuros/kaunta/commit/be62fc72970727691df51483e34d54001e8e2488))

## [0.14.6](https://github.com/seuros/kaunta/compare/v0.14.5...v0.14.6) (2025-11-10)


### Bug Fixes

* enable CORS credentials for cookie auth ([88b7f44](https://github.com/seuros/kaunta/commit/88b7f446a8632d4d1a0ae6837b4f963985aea22c))

## [0.14.5](https://github.com/seuros/kaunta/compare/v0.14.4...v0.14.5) (2025-11-10)


### Bug Fixes

* add credentials include to login fetch ([cd45a8b](https://github.com/seuros/kaunta/commit/cd45a8bcc0eb8592ca8ce1791b6aca4beb09eb4b))

## [0.14.4](https://github.com/seuros/kaunta/compare/v0.14.3...v0.14.4) (2025-11-10)


### Bug Fixes

* include credentials in login fetch request for CSRF validation ([d51c56f](https://github.com/seuros/kaunta/commit/d51c56f45476dbd02b36d29fdd277cc05099a488))

## [0.14.3](https://github.com/seuros/kaunta/compare/v0.14.2...v0.14.3) (2025-11-10)


### Bug Fixes

* read CSRF token from cookie instead of server injection ([642cf05](https://github.com/seuros/kaunta/commit/642cf05b709ce25a2159b401f19a835e9497a686))

## [0.14.2](https://github.com/seuros/kaunta/compare/v0.14.1...v0.14.2) (2025-11-09)


### Bug Fixes

* remove redundant CSRF endpoint and consolidate Fiber config ([a0ac37e](https://github.com/seuros/kaunta/commit/a0ac37ef2446cf8ad9556c627a6521e40a1f7b6a))

## [0.14.1](https://github.com/seuros/kaunta/compare/v0.14.0...v0.14.1) (2025-11-09)


### Bug Fixes

* use csrf.TokenFromContext in /api/auth/csrf endpoint ([4179df6](https://github.com/seuros/kaunta/commit/4179df66c0e6fdbbfcdacf7ebcbfe8ee269d55b6))

## [0.14.0](https://github.com/seuros/kaunta/compare/v0.13.2...v0.14.0) (2025-11-09)


### Features

* upgrade Fiber from v2 to v3 ([d901166](https://github.com/seuros/kaunta/commit/d90116665da61255586fb8ffef1d705042a33330))

## [0.13.2](https://github.com/seuros/kaunta/compare/v0.13.1...v0.13.2) (2025-11-09)


### Bug Fixes

* use csrf.TokenFromContext() to properly extract CSRF token ([fcbb643](https://github.com/seuros/kaunta/commit/fcbb643972700718baccb957a07cf602b649a2c0))

## [0.13.1](https://github.com/seuros/kaunta/compare/v0.13.0...v0.13.1) (2025-11-09)


### Bug Fixes

* safe type assertion for CSRF token extraction ([684b077](https://github.com/seuros/kaunta/commit/684b0770d0e347546feb7483ff3e6c4afdeea145))

## [0.13.0](https://github.com/seuros/kaunta/compare/v0.12.0...v0.13.0) (2025-11-09)


### Features

* **auth:** add redirect middleware and fix CSRF configuration ([#20](https://github.com/seuros/kaunta/issues/20)) ([9ff5df3](https://github.com/seuros/kaunta/commit/9ff5df3017ce592792e3ebefa07ab1be5c2080e5))

## [0.12.0](https://github.com/seuros/kaunta/compare/v0.11.2...v0.12.0) (2025-11-09)


### Features

* enable prefork mode for bare metal, disable for Docker ([92d97e7](https://github.com/seuros/kaunta/commit/92d97e7a9e5568dba9d6e514d7fa57e86e7328d5))

## [0.11.2](https://github.com/seuros/kaunta/compare/v0.11.1...v0.11.2) (2025-11-09)


### Bug Fixes

* CSRF KeyLookup format for Fiber v2.52.9 compatibility ([d7406be](https://github.com/seuros/kaunta/commit/d7406be0792b43a01260519b9d2ea586a2262a3c))

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
