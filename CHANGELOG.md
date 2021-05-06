# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [1.1.0] - 2021-05-07

### Added

- Added `miner` label containing the miner address to all miner metrics (already present as `instance` in typical setups).
- Added per-second income metrics.
- Added metadata metrics for pools, containing the pool name and currency.

### Fixed

- Fixed the magnitude of the unpaid and unconfirmed coins metrics (e.g. convert from wei to ether).

## [1.0.0] - 2021-05-02

Initial release.
