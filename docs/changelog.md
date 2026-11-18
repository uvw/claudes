# Changelog

## [0.6.0](https://github.com/uvw/claudes/compare/v0.5.0...v0.6.0) (2026-11-18)


### Features

* **cli:** filter session ls by running state, and take today, yesterday, week, and month for durations ([#106](https://github.com/uvw/claudes/issues/106)) ([63f5d5b](https://github.com/uvw/claudes/commit/63f5d5b0bd7b19d0b1b4ddcf2b60c77322e983f6))


### Bug fixes

* **cli:** cut long branches and fit names to the terminal in every table ([#98](https://github.com/uvw/claudes/issues/98)) ([c4a081f](https://github.com/uvw/claudes/commit/c4a081f9c70e689b11bc4087e3cf8e202617ae26))
* **cli:** show session ls's branch column only with --wide ([#100](https://github.com/uvw/claudes/issues/100)) ([11b58db](https://github.com/uvw/claudes/commit/11b58db6f5311b88a89f3276826afe77b2052628))

## [0.5.0](https://github.com/uvw/claudes/compare/v0.4.0...v0.5.0) (2026-11-18)

### Features

- **cli:** add short forms for the flags typed most often
  ([#69](https://github.com/uvw/claudes/issues/69))
  ([879328d](https://github.com/uvw/claudes/commit/879328d89e14e5bcea333a419909e6c0d6a9cb32))
- **cli:** carry a stopped background job over in session mv
  ([#73](https://github.com/uvw/claudes/issues/73))
  ([b353c7f](https://github.com/uvw/claudes/commit/b353c7f8b317b25be24d1ef5793ddde47812c390))
- **cli:** list the MCP servers a session or directory has with claudes mcp ls
  ([#81](https://github.com/uvw/claudes/issues/81))
  ([abca868](https://github.com/uvw/claudes/commit/abca8683d7d51d8359ec252daa6690f204dbfb52))
- **cli:** mask secrets in session search and rename --limit to --top
  ([#91](https://github.com/uvw/claudes/issues/91))
  ([65c8da0](https://github.com/uvw/claudes/commit/65c8da08a0d769456d1401cbdfef1d7e56d92475))
- **cli:** search past sessions by what was said in them
  ([#78](https://github.com/uvw/claudes/issues/78))
  ([6f53734](https://github.com/uvw/claudes/commit/6f53734fe0f0ddb76ffe1b6e68cf8962fa0c534b))
- **cli:** shorten long directories to fit every text table
  ([#85](https://github.com/uvw/claudes/issues/85))
  ([c67d2ff](https://github.com/uvw/claudes/commit/c67d2ffb94ef061d64ea82239dee9aa72f65c031))
- **cli:** show running subagents in ls, and more with --wide
  ([#74](https://github.com/uvw/claudes/issues/74))
  ([3667368](https://github.com/uvw/claudes/commit/36673681610c52714678807b4517a45449e25c62))
- **cli:** warn in doctor when Claude Code may write pins without the lock
  ([#66](https://github.com/uvw/claudes/issues/66))
  ([35539a0](https://github.com/uvw/claudes/commit/35539a0cc847715b8be159465f0e3a80b0b58497))
- **procs:** read process start times from /proc on linux
  ([#67](https://github.com/uvw/claudes/issues/67))
  ([280c852](https://github.com/uvw/claudes/commit/280c85208e8315db7cef412882f7bef29953aced))
- **safefile:** replace open files with posix rename semantics on windows
  ([#75](https://github.com/uvw/claudes/issues/75))
  ([85b3415](https://github.com/uvw/claudes/commit/85b34153d5df2064adc45d9f555acec09b3d622f))

### Bug fixes

- **claude:** drop the child-notes and effort variables when starting claude
  ([#84](https://github.com/uvw/claudes/issues/84))
  ([aa286f0](https://github.com/uvw/claudes/commit/aa286f0d5821a948c9d16ed6ce975b7ea5e2a9fa))
- **cli:** add --confirm to gc and session gc, and lay out cli by command
  ([#92](https://github.com/uvw/claudes/issues/92))
  ([d33ab4b](https://github.com/uvw/claudes/commit/d33ab4b7f5d28261233de5e559727e62bad876dd))
- **cli:** close safety gaps in gc, session mv, pause, and their hints
  ([#89](https://github.com/uvw/claudes/issues/89))
  ([2d59df4](https://github.com/uvw/claudes/commit/2d59df45bbbc78425bbda7bd249cdd025acc71ee))
- **cli:** trash what gc --teams and session mv --rm remove, and survive SIGTERM
  and a suspend ([#95](https://github.com/uvw/claudes/issues/95))
  ([2a2b981](https://github.com/uvw/claudes/commit/2a2b981a6794e3d3288a85f8d5d0afd9f58fe0c3))
- **deps:** Bump charm.land/lipgloss/v2 from 2.0.6 to 2.1.0 in the gomod group
  across 1 directory ([#76](https://github.com/uvw/claudes/issues/76))
  ([596aa8f](https://github.com/uvw/claudes/commit/596aa8f27e8c2a209053de82eeabdbf1244678e9))
- **hack:** date the names of the release tokens so a renewal can be made
  ([#94](https://github.com/uvw/claudes/issues/94))
  ([b611df5](https://github.com/uvw/claudes/commit/b611df550d5190138cd55f63c2dffb1ba64f3414))
- judge sessions only on records claudes knows, and harden moves and renames
  ([#90](https://github.com/uvw/claudes/issues/90))
  ([06ce4c7](https://github.com/uvw/claudes/commit/06ce4c7e4a26a910c0075cf71277951bd2250ad6))
- **procs:** bound the start time read from /proc
  ([#80](https://github.com/uvw/claudes/issues/80))
  ([300a1ee](https://github.com/uvw/claudes/commit/300a1eea112b0d7abb398e0549475e64cd2a98c3))
- **procs:** refuse a process that started after its session
  ([#70](https://github.com/uvw/claudes/issues/70))
  ([79d3cb1](https://github.com/uvw/claudes/commit/79d3cb1fddbba5368945d0f87ea4f7c215af3fa3))
- **procs:** stop an npm-installed claude code on windows
  ([#71](https://github.com/uvw/claudes/issues/71))
  ([6f5a390](https://github.com/uvw/claudes/commit/6f5a39040d05d0be1cf2bb8ec694795b71339ed4))
- **style:** follow the clicolors convention, and check Claude Code's formats
  weekly ([#93](https://github.com/uvw/claudes/issues/93))
  ([ddacc3d](https://github.com/uvw/claudes/commit/ddacc3dd9210b5f483c7164533c6951b8cd03b5d))

## [0.4.0](https://github.com/uvw/claudes/compare/v0.3.0...v0.4.0) (2026-11-17)

### Features

- **cli:** give every table the same column order and cut long names
  ([#62](https://github.com/uvw/claudes/issues/62))
  ([1476a8a](https://github.com/uvw/claudes/commit/1476a8afb3db1ffcc1c7794ad29d4e3cff37fc20))
- **cli:** list the biggest sessions with session du --top
  ([#57](https://github.com/uvw/claudes/issues/57))
  ([c3f189c](https://github.com/uvw/claudes/commit/c3f189cf08e8e60b1058eca1e25984216f1850bc))
- **cli:** remove sessions that did nothing with session gc --trivial
  ([#54](https://github.com/uvw/claudes/issues/54))
  ([7197cdc](https://github.com/uvw/claudes/commit/7197cdc4a6003f1426e4793c57e07a6973e30aea))
- **cli:** sort past sessions into remove, review, and keep with session triage
  ([#55](https://github.com/uvw/claudes/issues/55))
  ([151ea59](https://github.com/uvw/claudes/commit/151ea590bf759ae68ec6bddebd57eee53a6dae26))
- **cli:** trim saved outputs and old subagents with session gc --sidecars
  ([#58](https://github.com/uvw/claudes/issues/58))
  ([232f18f](https://github.com/uvw/claudes/commit/232f18f2bac3328baf9ec98f2bca23958988c415))
- say directory instead of folder everywhere
  ([#63](https://github.com/uvw/claudes/issues/63))
  ([0923012](https://github.com/uvw/claudes/commit/092301253553f2b9773eac2951267732acdd90aa))

## [0.3.0](https://github.com/uvw/claudes/compare/v0.2.1...v0.3.0) (2026-11-16)

### Features

- **cli:** take pinned sessions in bulk with --pinned
  ([#51](https://github.com/uvw/claudes/issues/51))
  ([47c7a77](https://github.com/uvw/claudes/commit/47c7a77f217e494214b8f22432f18b14614ba0d1))

### Bug fixes

- **cli:** read a running session's version from the binary it runs
  ([#52](https://github.com/uvw/claudes/issues/52))
  ([46fc510](https://github.com/uvw/claudes/commit/46fc510168ee868b8357e02dabbca6a65ba19daf))

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
