# Changelog

## [0.2.1](https://github.com/uvw/claudes/compare/v0.2.0...v0.2.1) (2026-11-16)

### Features

- **cli:** answer to t and teams for claudes team
  ([#45](https://github.com/uvw/claudes/issues/45))
  ([1a0ffda](https://github.com/uvw/claudes/commit/1a0ffdaa0821c85b7fcd04722b33b759c6586eb2))

### Bug fixes

- **safefile:** wait for a lock that another process is removing on windows
  ([#48](https://github.com/uvw/claudes/issues/48))
  ([3a97557](https://github.com/uvw/claudes/commit/3a975572711fcd4a2d0a3592634a4e4502d358ee))

## [0.2.0](https://github.com/uvw/claudes/compare/v0.1.0...v0.2.0) (2026-11-16)

### Features

- **cli:** add claudes init fish, so claudes cd works in fish
  ([#37](https://github.com/uvw/claudes/issues/37))
  ([d13884c](https://github.com/uvw/claudes/commit/d13884c9c6ffa815c4a3c2941daff1a11b865141))
- **cli:** answer to s and sessions for claudes session
  ([#27](https://github.com/uvw/claudes/issues/27))
  ([6e0305d](https://github.com/uvw/claudes/commit/6e0305d56131361fae6432839e149198776fe238))
- **cli:** remove empty, copied, and leftover sessions with session gc
  ([#29](https://github.com/uvw/claudes/issues/29))
  ([dc066d4](https://github.com/uvw/claudes/commit/dc066d4cb699d6aeb6c2a781763d8fe9ae2820cb))
- **cli:** show how much disk space sessions take
  ([#28](https://github.com/uvw/claudes/issues/28))
  ([69b612e](https://github.com/uvw/claudes/commit/69b612e865b5777c6cdd78db365b3fedb34db41d))
- **cli:** upgrade claudes in place with claudes upgrade
  ([#31](https://github.com/uvw/claudes/issues/31))
  ([5219179](https://github.com/uvw/claudes/commit/52191798086fa19922ed3c5e72a8ade48c259940))
- **release:** install claudes with one command on macos, linux, and windows
  ([#30](https://github.com/uvw/claudes/issues/30))
  ([02edb5c](https://github.com/uvw/claudes/commit/02edb5c5f886356f3bb2e65cbc0f3fe9ddaa068a))
- **release:** publish each release to homebrew and winget
  ([#32](https://github.com/uvw/claudes/issues/32))
  ([b21f802](https://github.com/uvw/claudes/commit/b21f802196a745fe850ad07bf03ba72a8aa95b58))

### Bug fixes

- **e2e:** report a failed close of the fake's call log
  ([#35](https://github.com/uvw/claudes/issues/35))
  ([a26c70d](https://github.com/uvw/claudes/commit/a26c70d55c2571a9cc201600c042b450e1cca8f0))
- **release:** find the release pull request when formatting its changelog
  ([#43](https://github.com/uvw/claudes/issues/43))
  ([f5e2a83](https://github.com/uvw/claudes/commit/f5e2a83a49af68617d4f0c62167764c3689f59d1))

## 0.1.0 (2026-11-15)

### Features

- add paths, atomic files, quoting, process control, and matching
  ([ce658dd](https://github.com/uvw/claudes/commit/ce658ddb673491c5df6e572f9b9d42e8c0dd5aa0))
- check Claude Code compatibility, and read transcripts and agent teams
  ([b730fdb](https://github.com/uvw/claudes/commit/b730fdb455b4600d3c394bb40811f82a602213ae))
- **claude:** reach the claude command line through one client
  ([fbdc340](https://github.com/uvw/claudes/commit/fbdc340bf2125d8be04e654463d54e56a3feb622))
- **cli:** add the claudes commands and the agent skill
  ([dd1c446](https://github.com/uvw/claudes/commit/dd1c446029217cb50d423f3b4d6edf2946e8300a))
- read and write agent view's jobs and pins, the park file, and config.yaml
  ([da2d051](https://github.com/uvw/claudes/commit/da2d0511a9384b491b7e95e0c0bf6fe3106f7a9e))

### Bug fixes

- **cli:** keep --verbose from passing through, and report refusals and failures
  in full ([#24](https://github.com/uvw/claudes/issues/24))
  ([373aa75](https://github.com/uvw/claudes/commit/373aa75ddcb5982add13001b9a226e90fc123152))
- **cli:** match sessions whose folder was recorded through a link
  ([5a121e5](https://github.com/uvw/claudes/commit/5a121e5fd4a8c8b221f96e21e705c7da1eb93fe7))
