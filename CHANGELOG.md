# Changelog

## [0.26.0](https://github.com/seuros/kaunta/compare/v0.25.0...v0.26.0) (2025-11-27)


### Features

* add doctor CLI command for health checks ([#80](https://github.com/seuros/kaunta/issues/80)) ([03ea0ef](https://github.com/seuros/kaunta/commit/03ea0efaab3540b2c7e0e22beccc3c1d60ca7ed3))
* add website management dashboard UI ([#78](https://github.com/seuros/kaunta/issues/78)) ([1201025](https://github.com/seuros/kaunta/commit/12010257f3982834cc90b357c1c0a28626226563))

## [0.25.0](https://github.com/seuros/kaunta/compare/v0.24.2...v0.25.0) (2025-11-27)


### Features

* add automated install script for Unix platforms ([9f2f2e2](https://github.com/seuros/kaunta/commit/9f2f2e235cb884b87a1d7406415b42624d9729c5))
* add binary build and Docker push to release-please workflow ([102e696](https://github.com/seuros/kaunta/commit/102e69667da952a8c1c090d32d4ca912c6484f5e))
* add branding, favicon, and Open Graph meta tags ([#48](https://github.com/seuros/kaunta/issues/48)) ([337e3be](https://github.com/seuros/kaunta/commit/337e3bec085f37e65dad25f08b4076e91e83a0cb))
* Add comprehensive CLI for website and analytics management ([d8239ce](https://github.com/seuros/kaunta/commit/d8239ceb2d92744464134eb3d0aa6888cdeed230))
* add CSRF protection and rate limiting to login endpoint ([e922c80](https://github.com/seuros/kaunta/commit/e922c80e81c0dfdc6b9128867d1612d49161ca23))
* add CSRF token cleanup on logout ([3909531](https://github.com/seuros/kaunta/commit/39095319698648cdb56d2d8ecd9974ea52e8330b))
* add entry/exit page tracking with dashboard tabs ([#77](https://github.com/seuros/kaunta/issues/77)) ([d4f3b0f](https://github.com/seuros/kaunta/commit/d4f3b0f19963cdff4c36ff8823492f79c478f2fa))
* add FreeBSD build support with Deno fallback ([5c93afb](https://github.com/seuros/kaunta/commit/5c93afb39783056fd8599b5d993868128bb909cc)), closes [#47](https://github.com/seuros/kaunta/issues/47)
* add pagination support to list endpoints ([47aa232](https://github.com/seuros/kaunta/commit/47aa23244c4e0380fa411d271b6bcc87382a9742))
* add real-time WebSocket updates and fix dashboard filters ([fb739a4](https://github.com/seuros/kaunta/commit/fb739a4d84529c5737ff74d2346175a5e609c198))
* add self-update functionality ([4a07c66](https://github.com/seuros/kaunta/commit/4a07c660dcf5b0fe3608f31604e6cd15ca4dc9f3))
* add sorting support to analytics endpoints ([#75](https://github.com/seuros/kaunta/issues/75)) ([0c7f2cc](https://github.com/seuros/kaunta/commit/0c7f2cc9133932256185f211c3231d7b667819b3))
* add test files and CLI improvements ([f8002a9](https://github.com/seuros/kaunta/commit/f8002a9157ef460a59b2ad2ec71c75e28e3e4002))
* add TOML config file support ([1f1cb4b](https://github.com/seuros/kaunta/commit/1f1cb4b13fdc86ab108c6ded3fcc0921b9289987))
* add trusted origins config sync and non-interactive password reset ([4a14e36](https://github.com/seuros/kaunta/commit/4a14e365efebbd34f98e6c67343e20262aac8c0d))
* add username-based authentication system ([75cf106](https://github.com/seuros/kaunta/commit/75cf106fa05a9b6e84e46525ee4af988c8eed780))
* add UTM campaign parameter tracking ([#76](https://github.com/seuros/kaunta/issues/76)) ([c9326f1](https://github.com/seuros/kaunta/commit/c9326f11cee52057d12023171121b75fcbe417d0))
* add web-based setup wizard for initial configuration ([#64](https://github.com/seuros/kaunta/issues/64)) ([602d658](https://github.com/seuros/kaunta/commit/602d65849fa313e072eca68ad422201a8039dd95))
* **auth:** add redirect middleware and fix CSRF configuration ([#20](https://github.com/seuros/kaunta/issues/20)) ([5509b40](https://github.com/seuros/kaunta/commit/5509b40a23e68b117b4be555f798cd038b654f81))
* auto-allow domain variations on website creation ([ed62618](https://github.com/seuros/kaunta/commit/ed6261899e9c2955e7129cd90e4dffebef6b359d))
* cleaner trusted origin ([183932c](https://github.com/seuros/kaunta/commit/183932c4de7dcb227f39ccabb322a6f8563d6181))
* create dedicated map page with TopoJSON integration ([28266f8](https://github.com/seuros/kaunta/commit/28266f84b156f6fb2a6be580e7a81c0fe91fbe9b))
* create dedicated map page with TopoJSON integration ([d857286](https://github.com/seuros/kaunta/commit/d857286fb7b026f1e8e36bc8b98b8d4725250609))
* enable prefork mode for bare metal, disable for Docker ([db1ad7e](https://github.com/seuros/kaunta/commit/db1ad7ef21198d0db47f0788f11ca13b90f751ff))
* fix defer/async script loading and add Playwright tests ([d15c82f](https://github.com/seuros/kaunta/commit/d15c82f0cc9ee4a5af40a9d9f831cf5f6093e13b))
* fix tracker initialization and dynamic ETag generation ([f359c1e](https://github.com/seuros/kaunta/commit/f359c1eebe192c2cb282e1689908ca7e75793bb5))
* implement Fiber template engine with layout system ([6c0ef3d](https://github.com/seuros/kaunta/commit/6c0ef3d71447cb48060bef122d7c755da8fadec0))
* improve Docker workflow and fix dashboard issues ([37474b0](https://github.com/seuros/kaunta/commit/37474b0e41eabc9b774944de9796fa0f53e626a1))
* migrate analytics queries to PostgreSQL functions ([d85734f](https://github.com/seuros/kaunta/commit/d85734f84cb8f949bdb471c33a7dc6839a77f3c7))
* optimize tracker with rAF scroll batching, ResizeObserver, sendBeacon, AbortController cleanup, and test infrastructure ([105511e](https://github.com/seuros/kaunta/commit/105511ed828a70fa833479e6683a55674cff63e2))
* redesign home page and dashboard ([#9](https://github.com/seuros/kaunta/issues/9)) ([95fc376](https://github.com/seuros/kaunta/commit/95fc376d07b1b82e85a89a2fe24346ff35bd7460))
* redesign the login page ([#10](https://github.com/seuros/kaunta/issues/10)) ([de2dccb](https://github.com/seuros/kaunta/commit/de2dccbd49d9b874335c8eb8f639991bcd8f37bd))
* switch zap logger ([ed62618](https://github.com/seuros/kaunta/commit/ed6261899e9c2955e7129cd90e4dffebef6b359d))
* upgrade Fiber from v2 to v3 ([1898909](https://github.com/seuros/kaunta/commit/189890980f1f36a319985f34908760684f1da0b5))
* Use XDG paths. ([#60](https://github.com/seuros/kaunta/issues/60)) ([321866d](https://github.com/seuros/kaunta/commit/321866de3bfc3df67b3fc590340646b328a3b178))


### Bug Fixes

* add assets ([2f0390f](https://github.com/seuros/kaunta/commit/2f0390f56f5be26798ef5659df40cd77df2c4aed))
* add checkout step to upload-release-assets job ([63ffe95](https://github.com/seuros/kaunta/commit/63ffe9506472d133ea635b176348a4b8f057dfc7))
* add credentials include to login fetch ([c88e498](https://github.com/seuros/kaunta/commit/c88e498efcc0f140e520405522a31733ad33bd33))
* add debug ([3befb5c](https://github.com/seuros/kaunta/commit/3befb5c5f000473870a95daf3bf4dc633cfcfc28))
* add POST to CORS allowed methods for tracking API ([3825051](https://github.com/seuros/kaunta/commit/3825051635d34d145465274c2e3c4dc196a34730))
* Alpine.js duplicate key errors and filter data extraction ([605201b](https://github.com/seuros/kaunta/commit/605201b51003950756369b380f4a8c237be9315d))
* auto-detect HTTPS via reverse proxy headers ([11aea35](https://github.com/seuros/kaunta/commit/11aea35a3b1b1dea021f605bd2933898aa7217ff))
* build ([45ad174](https://github.com/seuros/kaunta/commit/45ad17412cd9556f62fafe7f0be56a330a292713))
* bump version ([6af73b2](https://github.com/seuros/kaunta/commit/6af73b2fe4d135ae4265bf74d6087a85f312b8cf))
* cookie SameSite policy for HTTPS proxy compatibility ([cf2dad5](https://github.com/seuros/kaunta/commit/cf2dad50a4db5c1c5a79d854b9bc8c709909d349))
* create global CSS file and optimize styles ([#71](https://github.com/seuros/kaunta/issues/71)) ([04ef718](https://github.com/seuros/kaunta/commit/04ef71886bd512a7e1e8286071faefdf1f492ad9))
* Critical production bugs and linter compliance ([1528ff5](https://github.com/seuros/kaunta/commit/1528ff5ffb056c8231a90bb945af5461dfaf1932))
* CSRF KeyLookup format for Fiber v2.52.9 compatibility ([3875f29](https://github.com/seuros/kaunta/commit/3875f298bd205aa99fad08d361a48610aec3f28e))
* **dashboard:** resolve chart rendering race condition ([#16](https://github.com/seuros/kaunta/issues/16)) ([5468140](https://github.com/seuros/kaunta/commit/54681407388b06a21b836c16bf9f1087034f12ba))
* disable secure cookies in docker-compose for local dev ([cfb4a67](https://github.com/seuros/kaunta/commit/cfb4a67a3563439ef027000acc92515e426da19b))
* enable CORS credentials for cookie auth ([f0a9fbb](https://github.com/seuros/kaunta/commit/f0a9fbb5c6db77eab1366803e0712e0a6a52b7cb))
* extract index outside ([f9b178b](https://github.com/seuros/kaunta/commit/f9b178b617005818e8e0321a0bfc94e6cd4f891c))
* fix release ([2d17b10](https://github.com/seuros/kaunta/commit/2d17b107cb7684819ca22539fd628421a4475d49))
* handle empty IP address in session creation ([7ad61cc](https://github.com/seuros/kaunta/commit/7ad61cc87c5510b06608360c26fd485c6f6680da))
* hide self-upgrade flags in dev builds ([2a9effb](https://github.com/seuros/kaunta/commit/2a9effb62fed661c8c65919865eddf3d1fd1ed66))
* improve Docker healthcheck to test service health ([4421b76](https://github.com/seuros/kaunta/commit/4421b7657fabc74ca85ffc32266695667178cf00))
* improve map initialization with proper cleanup and retry logic ([#68](https://github.com/seuros/kaunta/issues/68)) ([f29989f](https://github.com/seuros/kaunta/commit/f29989f55dfbf0c44ee7a2b1156a1aa296220fd3))
* include credentials in login fetch request for CSRF validation ([dde02c9](https://github.com/seuros/kaunta/commit/dde02c955c4a1d5016a86450d7d9b824f817c061))
* init release ([609243d](https://github.com/seuros/kaunta/commit/609243d96ec01a323d3c645bc7d8a634ca135012))
* introduce structured logging ([2d71501](https://github.com/seuros/kaunta/commit/2d715015c32c7804d0aa0b34945e55d9feccb9d7))
* make CSRF cookies work without HTTPS ([c192422](https://github.com/seuros/kaunta/commit/c19242232e5a2d5dade8d009f92599643f94fbe8))
* move binary to /usr/local/bin and add Docker user management docs ([40fab37](https://github.com/seuros/kaunta/commit/40fab370bdca76adf866467325dc797e58fa172d)), closes [#53](https://github.com/seuros/kaunta/issues/53)
* push fixing migration ([ea1923e](https://github.com/seuros/kaunta/commit/ea1923e69a95ff135b219a79f0ab331fe58c2ec7))
* read CSRF token from cookie instead of server injection ([9de7b4c](https://github.com/seuros/kaunta/commit/9de7b4c477a6d985d0643e2357e9a97c9979dec7))
* Remove broken tests.yml and add linting to test.yml ([7f2f1af](https://github.com/seuros/kaunta/commit/7f2f1aff106544d8ff4b523dd883e0d389c0aab7))
* remove redundant CSRF endpoint and consolidate Fiber config ([5e43fe4](https://github.com/seuros/kaunta/commit/5e43fe44b1f1c915d168a6687bf722a0823d8f57))
* render empty chart and fix null array responses ([399228f](https://github.com/seuros/kaunta/commit/399228fb7a4bee802b44c1945af78f6eb40a2c02))
* resolve race conditions in realtime package ([23648c1](https://github.com/seuros/kaunta/commit/23648c13f161f98348efe09cd847e8f392c16e41))
* resolve tracking endpoint failures - CHAR(2) country column constraint ([be074b9](https://github.com/seuros/kaunta/commit/be074b91a06822358fe13540b91dd172f8a4fe57))
* safe type assertion for CSRF token extraction ([8976454](https://github.com/seuros/kaunta/commit/8976454a6a21eb2ea88fffdeb04a97066802e45b))
* self-upgrade runs before database validation ([6240c5e](https://github.com/seuros/kaunta/commit/6240c5e1a4a90e7de75ff309b05a570a902f3e30))
* Show full website IDs in table format instead of truncated ([55cc9f8](https://github.com/seuros/kaunta/commit/55cc9f83d1cc09316c0581017afda5fb2d784fe3))
* skip CSRF for static assets (JS/CSS) to prevent cross-site cookie rejection ([#63](https://github.com/seuros/kaunta/issues/63)) ([99a279a](https://github.com/seuros/kaunta/commit/99a279a1796955898d0eb7feeec35b60fe56341c))
* tag generation ([260226d](https://github.com/seuros/kaunta/commit/260226d2f4282530022be02c22bb4d8d8340e402))
* test ci ([b9c8618](https://github.com/seuros/kaunta/commit/b9c8618fbbffacc012852892d1cc54ad0c42857a))
* tidy go module dependencies ([3c9383f](https://github.com/seuros/kaunta/commit/3c9383f7167b073ea2d9da00482147a331edfac7))
* update GeoIP database URL and clean up version API response ([79ae811](https://github.com/seuros/kaunta/commit/79ae811bb3e6bc9b1150858d086d3ae34aed7545))
* update js to remove defensive code ([289a29d](https://github.com/seuros/kaunta/commit/289a29d458d273ef4dcb0bf2001633932d083ed2))
* update release workflow for self-upgrade support ([95d14db](https://github.com/seuros/kaunta/commit/95d14db9a5c3b46ad246e666a0525899ef7bf899))
* use AllowOriginsFunc for CORS credentials ([ae5a2b7](https://github.com/seuros/kaunta/commit/ae5a2b7dc683e99f0e7735e147373b9422782e68))
* use bun to compile js ([5d448ef](https://github.com/seuros/kaunta/commit/5d448ef94870d5024dfd9b24f72b554925e9f717))
* use csrf.TokenFromContext in /api/auth/csrf endpoint ([5ec3e29](https://github.com/seuros/kaunta/commit/5ec3e29b2da0fdbb30e356ed6248735c7296de00))
* use csrf.TokenFromContext() to properly extract CSRF token ([bb0aaae](https://github.com/seuros/kaunta/commit/bb0aaae06d413b630bdf43443e33bb8fa2166583))
* use PersistentPreRunE to ensure config file loading works ([f6cf597](https://github.com/seuros/kaunta/commit/f6cf597722c5fa5a1478abbdc53af1d891ed91f4))
* use SameSite=None for CSRF cookie ([a19600f](https://github.com/seuros/kaunta/commit/a19600faa65ce16c76d63fc55c344e85883bb977))
* use TRUSTED_ORIGINS env var for CSRF with proxy ([7234f7f](https://github.com/seuros/kaunta/commit/7234f7f581dbc443d1870e8425a66dcdf2d50f0c))
* use X-Forwarded-For for client IP detection behind reverse proxy ([bcd184c](https://github.com/seuros/kaunta/commit/bcd184c5d5d5d265ee45e6deefbe8bb43369bf65))

## [0.24.2](https://github.com/seuros/kaunta/compare/v0.24.1...v0.24.2) (2025-11-20)


### Bug Fixes

* create global CSS file and optimize styles ([#71](https://github.com/seuros/kaunta/issues/71)) ([c60b1e8](https://github.com/seuros/kaunta/commit/c60b1e8d71c30c683ec6dc860c2977f2c1a2944d))

## [0.24.1](https://github.com/seuros/kaunta/compare/v0.24.0...v0.24.1) (2025-11-20)


### Bug Fixes

* improve map initialization with proper cleanup and retry logic ([#68](https://github.com/seuros/kaunta/issues/68)) ([96a8c36](https://github.com/seuros/kaunta/commit/96a8c36e9aa63accee4cbd085183e52be8ccda9a))

## [0.24.0](https://github.com/seuros/kaunta/compare/v0.23.1...v0.24.0) (2025-11-18)


### Features

* create dedicated map page with TopoJSON integration ([b6689e4](https://github.com/seuros/kaunta/commit/b6689e43dafbcbffe4269ea67114009450e074a2))
* create dedicated map page with TopoJSON integration ([979afa7](https://github.com/seuros/kaunta/commit/979afa7f762729838fe67eb33af9d666e201bd8a))
* implement Fiber template engine with layout system ([d1bb681](https://github.com/seuros/kaunta/commit/d1bb68153022bc4a042fc43067d3eea9c8154ba1))

## [0.23.1](https://github.com/seuros/kaunta/compare/v0.23.0...v0.23.1) (2025-11-17)


### Bug Fixes

* skip CSRF for static assets (JS/CSS) to prevent cross-site cookie rejection ([#63](https://github.com/seuros/kaunta/issues/63)) ([eb103a7](https://github.com/seuros/kaunta/commit/eb103a76105da5847c594c9dd8877a35e60ec5ce))

## [0.23.0](https://github.com/seuros/kaunta/compare/v0.22.2...v0.23.0) (2025-11-17)


### Features

* Use XDG paths. ([#60](https://github.com/seuros/kaunta/issues/60)) ([5cfe214](https://github.com/seuros/kaunta/commit/5cfe214336ee6ae5ad1680eaa2b64e6dd997a8a5))


### Bug Fixes

* Alpine.js duplicate key errors and filter data extraction ([cf622b4](https://github.com/seuros/kaunta/commit/cf622b40d5b717b11a75c98195b3405ad840f966))
* handle empty IP address in session creation ([4ff74f9](https://github.com/seuros/kaunta/commit/4ff74f9f1ccad75873ebc83756fd1fe9481ada20))

## [0.22.2](https://github.com/seuros/kaunta/compare/v0.22.1...v0.22.2) (2025-11-17)


### Bug Fixes

* self-upgrade runs before database validation ([e94c2eb](https://github.com/seuros/kaunta/commit/e94c2eb0afa32694292963669915e8c289be69e2))

## [0.22.1](https://github.com/seuros/kaunta/compare/v0.22.0...v0.22.1) (2025-11-17)


### Bug Fixes

* update js to remove defensive code ([9ee1172](https://github.com/seuros/kaunta/commit/9ee11721c5ccabf7b07294c3760951096e476afd))

## [0.22.0](https://github.com/seuros/kaunta/compare/v0.21.2...v0.22.0) (2025-11-16)


### Features

* add automated install script for Unix platforms ([bbf3f10](https://github.com/seuros/kaunta/commit/bbf3f1062bd74b1a110d41d70be2ab5063d5d45e))


### Bug Fixes

* use X-Forwarded-For for client IP detection behind reverse proxy ([f87a7ec](https://github.com/seuros/kaunta/commit/f87a7ec958072e62698ea6d28a4a2baf6bf8e422))

## [0.21.2](https://github.com/seuros/kaunta/compare/v0.21.1...v0.21.2) (2025-11-16)


### Bug Fixes

* cookie SameSite policy for HTTPS proxy compatibility ([f77cd92](https://github.com/seuros/kaunta/commit/f77cd92ec5cbc94ead4b17100315f267fff061c8))

## [0.21.1](https://github.com/seuros/kaunta/compare/v0.21.0...v0.21.1) (2025-11-16)


### Bug Fixes

* auto-detect HTTPS via reverse proxy headers ([91ccbb4](https://github.com/seuros/kaunta/commit/91ccbb4081fe0ef31542be6ef7f6c2ab148e930b))
* disable secure cookies in docker-compose for local dev ([dc9ac4e](https://github.com/seuros/kaunta/commit/dc9ac4eeb627949a0ed3c708df178b35631e51a4))
* resolve race conditions in realtime package ([a66adb2](https://github.com/seuros/kaunta/commit/a66adb2badb11bff5e7daae3841492e1433fb204))

## [0.21.0](https://github.com/seuros/kaunta/compare/v0.20.0...v0.21.0) (2025-11-15)


### Features

* add CSRF token cleanup on logout ([3d1033b](https://github.com/seuros/kaunta/commit/3d1033b980eda867aee9d93e4a154b1575b271ea))
* auto-allow domain variations on website creation ([02ca235](https://github.com/seuros/kaunta/commit/02ca2356aa9f74b2d8b9fe73a2d8bc73fc9da08b))
* switch zap logger ([02ca235](https://github.com/seuros/kaunta/commit/02ca2356aa9f74b2d8b9fe73a2d8bc73fc9da08b))


### Bug Fixes

* hide self-upgrade flags in dev builds ([468d8ca](https://github.com/seuros/kaunta/commit/468d8caf6b0b3e4cab0ea4ff9935a3fef1f7d11a))

## [0.20.0](https://github.com/seuros/kaunta/compare/v0.19.0...v0.20.0) (2025-11-14)


### Features

* add branding, favicon, and Open Graph meta tags ([#48](https://github.com/seuros/kaunta/issues/48)) ([4d1de1a](https://github.com/seuros/kaunta/commit/4d1de1ae9c6776fdedce3c2532fc8370dd63354b))


### Bug Fixes

* make CSRF cookies work without HTTPS ([7af65b1](https://github.com/seuros/kaunta/commit/7af65b112dacc025a9e11216c8eefb97b5f6bef1))
* move binary to /usr/local/bin and add Docker user management docs ([4337716](https://github.com/seuros/kaunta/commit/4337716e55463800c55ab5733a2214904a3d6ed8)), closes [#53](https://github.com/seuros/kaunta/issues/53)
* use PersistentPreRunE to ensure config file loading works ([bf06d84](https://github.com/seuros/kaunta/commit/bf06d84de266f95d54f4a66eedc5f9dbae7fa09f))

## [0.19.0](https://github.com/seuros/kaunta/compare/v0.18.1...v0.19.0) (2025-11-13)


### Features

* add FreeBSD build support with Deno fallback ([9e0f84a](https://github.com/seuros/kaunta/commit/9e0f84ae9462df048f57ff11db563964e47b46ba)), closes [#47](https://github.com/seuros/kaunta/issues/47)
* add trusted origins config sync and non-interactive password reset ([c3b476d](https://github.com/seuros/kaunta/commit/c3b476dadcf4a5988cfb06172d8f343d97fbe353))

## [0.18.1](https://github.com/seuros/kaunta/compare/v0.18.0...v0.18.1) (2025-11-12)


### Bug Fixes

* tag generation ([dea35dd](https://github.com/seuros/kaunta/commit/dea35ddb5abf19be3be94b807952c49c1a20e8c7))

## [0.18.0](https://github.com/seuros/kaunta/compare/v0.17.0...v0.18.0) (2025-11-12)


### Features

* add pagination support to list endpoints ([735db9a](https://github.com/seuros/kaunta/commit/735db9a83a10403115abe189bf4aa3f5f1f263da))


### Bug Fixes

* add assets ([3bdb1b9](https://github.com/seuros/kaunta/commit/3bdb1b95b51909fc91445b2754eb1d634d0aefce))
* push fixing migration ([67f0af2](https://github.com/seuros/kaunta/commit/67f0af256e6fb8d28d39c90be83cbdc293fd286c))

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
