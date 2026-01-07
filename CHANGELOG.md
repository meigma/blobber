# Changelog

## 1.0.0 (2026-01-07)


### Features

* add installer script for curl|sh installation ([#21](https://github.com/GilmanLab/blobber/issues/21)) ([0b70d59](https://github.com/GilmanLab/blobber/commit/0b70d59f2bc70c5c5550b36c8a0a9cac501aad08))
* add Nix flake for installation and development ([#20](https://github.com/GilmanLab/blobber/issues/20)) ([6013265](https://github.com/GilmanLab/blobber/commit/6013265cf7f075d342abccad52f4b0c3aa45f95d))
* **archive:** implement eStargz build, extract, and read operations ([#2](https://github.com/GilmanLab/blobber/issues/2)) ([d52b4aa](https://github.com/GilmanLab/blobber/commit/d52b4aaf3af84f429bff9fd3d8cf6db07a873f11))
* **cache:** implement caching layer with security fixes ([#8](https://github.com/GilmanLab/blobber/issues/8)) ([1f0ba9d](https://github.com/GilmanLab/blobber/commit/1f0ba9d6689f53e41d0cb98166b8ed9d82e7bd76))
* **ci:** add workflow_dispatch trigger to release workflow ([1e7d325](https://github.com/GilmanLab/blobber/commit/1e7d3252c1dc32d87c968cc6e00437b5a1a4e6a1))
* **client:** implement Client, Push, and Pull operations ([#4](https://github.com/GilmanLab/blobber/issues/4)) ([ad27c16](https://github.com/GilmanLab/blobber/commit/ad27c16962709ec2e189ac5e7aa2f59e107e2631))
* **client:** implement Client, Push, Pull with integration tests ([#5](https://github.com/GilmanLab/blobber/issues/5)) ([4edd103](https://github.com/GilmanLab/blobber/commit/4edd10340f1f3adbf4e4b1fab169b7541b9e6d39))
* **cli:** implement push, pull, list, and cat commands ([#7](https://github.com/GilmanLab/blobber/issues/7)) ([4a9c0e8](https://github.com/GilmanLab/blobber/commit/4a9c0e88284f15531b1c732ba6b58cc8510a688e))
* **profiling:** improve auth and profiling harness ([#11](https://github.com/GilmanLab/blobber/issues/11)) ([f33c5c8](https://github.com/GilmanLab/blobber/commit/f33c5c8704a01d8523f1e6bc447bff7535ced29e))
* **registry:** implement OCI registry operations with ORAS ([#3](https://github.com/GilmanLab/blobber/issues/3)) ([1fecf3c](https://github.com/GilmanLab/blobber/commit/1fecf3cf18b6aaa2c88f05d48c7c6b9f9db140cd))
* **safepath:** implement PathValidator for secure path validation ([#1](https://github.com/GilmanLab/blobber/issues/1)) ([eb29bd5](https://github.com/GilmanLab/blobber/commit/eb29bd5a7bf6525dd690c438641e72bbe4fae800))


### Bug Fixes

* **ci:** lowercase repository name for GHCR compatibility ([#12](https://github.com/GilmanLab/blobber/issues/12)) ([96c64f8](https://github.com/GilmanLab/blobber/commit/96c64f8a673902fbe07037e98b0aef7f4c1ea4b5))
* **ci:** use PAT for release-please to trigger release workflow ([8c89799](https://github.com/GilmanLab/blobber/commit/8c89799fa4265f9175b4b8acea4fc01c38b0a5ba))
* **release:** use simple v* tags without component prefix ([26631aa](https://github.com/GilmanLab/blobber/commit/26631aaa0f6971c41400e534f537b9bf17ade0a4))
* **safepath:** reject absolute symlink targets ([#19](https://github.com/GilmanLab/blobber/issues/19)) ([15630a7](https://github.com/GilmanLab/blobber/commit/15630a7fd124b7aa4d5e983578411bc4423bd866))


### Code Refactoring

* code quality improvements for library release ([#6](https://github.com/GilmanLab/blobber/issues/6)) ([7c3b348](https://github.com/GilmanLab/blobber/commit/7c3b348d950aa18a1289b01f66c5fa5deaff662c))
* reduce technical debt and remove MVP prototype ([#10](https://github.com/GilmanLab/blobber/issues/10)) ([370370e](https://github.com/GilmanLab/blobber/commit/370370e80cce4afeb1f46154c4fb905e16f4cd95))
