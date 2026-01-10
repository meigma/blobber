# Changelog

## [2.0.0](https://github.com/meigma/blobber/compare/v1.1.0...v2.0.0) (2026-01-10)


### ⚠ BREAKING CHANGES

* Remove public interfaces (Registry, ArchiveBuilder, ArchiveReader, PathValidator, BlobHandle, BlobSource, Extractor) from core/types.go and type aliases from root package. Remove exported functions NewImageFromBlob and NewImageFromHandle.

### Code Refactoring

* move internal interfaces to internal/contracts ([#29](https://github.com/meigma/blobber/issues/29)) ([5b9d3dc](https://github.com/meigma/blobber/commit/5b9d3dcc3fdd93c1b77e6010767c8cb3597f92fa))

## [1.1.0](https://github.com/meigma/blobber/compare/v1.0.0...v1.1.0) (2026-01-09)


### Features

* add shell auto-completion for image file paths ([#17](https://github.com/meigma/blobber/issues/17)) ([227d232](https://github.com/meigma/blobber/commit/227d232e545a2fb70c79d71114df4bc1f9221c7b))
* add Sigstore signing and verification support ([#16](https://github.com/meigma/blobber/issues/16)) ([1b3b50a](https://github.com/meigma/blobber/commit/1b3b50a49ba075348cc5524860bbd34cba0bb22e))
* **cli:** add cp command, rename list to ls, improve auto-completion ([#18](https://github.com/meigma/blobber/issues/18)) ([db3142c](https://github.com/meigma/blobber/commit/db3142caaadbaf243be34bbb4279def47aa31e22))
* **cli:** improve ls output and add demo to landing page ([#21](https://github.com/meigma/blobber/issues/21)) ([e4de699](https://github.com/meigma/blobber/commit/e4de699b8d3ebae61e2fe447ef3c154012dec4ea))
* replace progress bars with charmbracelet library ([#24](https://github.com/meigma/blobber/issues/24)) ([6660479](https://github.com/meigma/blobber/commit/6660479b53c7654c22877100aea71ab33053bc67))


### Bug Fixes

* **docs:** convert absolute paths to relative paths ([#20](https://github.com/meigma/blobber/issues/20)) ([66c613a](https://github.com/meigma/blobber/commit/66c613a48dc10ca584a8b21d7be97192d83e5280))
* **install:** remove unnecessary -- separator from chmod ([e09ac69](https://github.com/meigma/blobber/commit/e09ac69075ffe3d06808de2b19593cebf091fece))
* **release:** migrate cosign signing to bundle format ([#25](https://github.com/meigma/blobber/issues/25)) ([45ff988](https://github.com/meigma/blobber/commit/45ff988dcb68de1cc2648598379c05a5db1f4ac3))

## [1.1.0](https://github.com/meigma/blobber/compare/v1.0.0...v1.1.0) (2026-01-09)


### Features

* add shell auto-completion for image file paths ([#17](https://github.com/meigma/blobber/issues/17)) ([227d232](https://github.com/meigma/blobber/commit/227d232e545a2fb70c79d71114df4bc1f9221c7b))
* add Sigstore signing and verification support ([#16](https://github.com/meigma/blobber/issues/16)) ([1b3b50a](https://github.com/meigma/blobber/commit/1b3b50a49ba075348cc5524860bbd34cba0bb22e))
* **cli:** add cp command, rename list to ls, improve auto-completion ([#18](https://github.com/meigma/blobber/issues/18)) ([db3142c](https://github.com/meigma/blobber/commit/db3142caaadbaf243be34bbb4279def47aa31e22))
* **cli:** improve ls output and add demo to landing page ([#21](https://github.com/meigma/blobber/issues/21)) ([e4de699](https://github.com/meigma/blobber/commit/e4de699b8d3ebae61e2fe447ef3c154012dec4ea))


### Bug Fixes

* **docs:** convert absolute paths to relative paths ([#20](https://github.com/meigma/blobber/issues/20)) ([66c613a](https://github.com/meigma/blobber/commit/66c613a48dc10ca584a8b21d7be97192d83e5280))
* **install:** remove unnecessary -- separator from chmod ([e09ac69](https://github.com/meigma/blobber/commit/e09ac69075ffe3d06808de2b19593cebf091fece))

## 1.0.0 (2026-01-08)


### Features

* add installer script for curl|sh installation ([c930979](https://github.com/meigma/blobber/commit/c930979e52fd43a684fc6eb0d922120aa6aa5464))
* add Nix flake for installation and development ([47f27db](https://github.com/meigma/blobber/commit/47f27dbca75d6084f4b8b1cc535eefb181de81aa))
* **archive:** implement eStargz build, extract, and read operations ([b56942f](https://github.com/meigma/blobber/commit/b56942f58dcb54256b8ac1a0deecfd9912414de1))
* **cache:** add TTL-based cache validation to skip manifest fetching ([f140d8b](https://github.com/meigma/blobber/commit/f140d8b66e0c543f539b380418dc168c9f704a76))
* **cache:** implement caching layer with security fixes ([c1e0297](https://github.com/meigma/blobber/commit/c1e029772911651ec03754dad7fca494837c2424))
* **ci:** add workflow_dispatch trigger to release workflow ([ac8b986](https://github.com/meigma/blobber/commit/ac8b98631aa55d0d6a503699b559de8083f91356))
* **cli:** add caching infrastructure with XDG paths and Viper config ([2b4ccd5](https://github.com/meigma/blobber/commit/2b4ccd529c748ce9e29831714b5113a591d9793a))
* **client:** implement Client, Push, and Pull operations ([d20373c](https://github.com/meigma/blobber/commit/d20373cb43ca1b47b05e5c7928148dd060ad336f))
* **client:** implement Client, Push, Pull with integration tests ([7b6ab2b](https://github.com/meigma/blobber/commit/7b6ab2b6460a0bef29ed5e625690c1bdc9106ff4))
* **cli:** implement push, pull, list, and cat commands ([de22a33](https://github.com/meigma/blobber/commit/de22a3355296e6f82daf58c1569d98756ad95cd5))
* **profiling:** improve auth and profiling harness ([d242475](https://github.com/meigma/blobber/commit/d2424755249cf4edda2880240c41aba647869fb0))
* **registry:** implement OCI registry operations with ORAS ([391d464](https://github.com/meigma/blobber/commit/391d464fa8772795c80e5bd631c96da8f5c0fbfb))
* **safepath:** implement PathValidator for secure path validation ([cd514f9](https://github.com/meigma/blobber/commit/cd514f9ef3cb7d70582f2a8dd0486e66b0748066))


### Bug Fixes

* **ci:** initialize cosign cache before parallel signing ([#7](https://github.com/meigma/blobber/issues/7)) ([730b1d7](https://github.com/meigma/blobber/commit/730b1d7862bb287e9ba58dcb383a1572c82fcac8))
* **ci:** lowercase repository name for GHCR compatibility ([8d17e98](https://github.com/meigma/blobber/commit/8d17e98309cfa6c4a26eb386d8e5e94fa010911d))
* **ci:** use PAT for release-please to trigger release workflow ([faa067a](https://github.com/meigma/blobber/commit/faa067a6c51b883176b18a19eae8454a2f99c56c))
* **release:** serialize cosign signing with flock ([#8](https://github.com/meigma/blobber/issues/8)) ([ad350d4](https://github.com/meigma/blobber/commit/ad350d4c3ff028b94073c9dad02a38eefb1dbcf1))
* **release:** use simple v* tags without component prefix ([68b54d5](https://github.com/meigma/blobber/commit/68b54d594462b1b0e5013c65dedb4212f5f9c280))
* **safepath:** reject absolute symlink targets ([80cc39e](https://github.com/meigma/blobber/commit/80cc39e7477d6a198d811d8f320c73860799ff3b))
* **test:** add delay before cache prune to fix timing race ([#2](https://github.com/meigma/blobber/issues/2)) ([53718ae](https://github.com/meigma/blobber/commit/53718ae019df3465d7b70868af22aa12665c2bb7))


### Code Refactoring

* code quality improvements for library release ([c3982ad](https://github.com/meigma/blobber/commit/c3982ad7cb20f2ef5fbe4435012dbdd686f0760e))
* reduce technical debt and remove MVP prototype ([c55ab23](https://github.com/meigma/blobber/commit/c55ab235cb260b44be30bb5de27f925d92699ae8))
