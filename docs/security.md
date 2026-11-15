# Security

## Supported versions

Only the latest release gets fixes.

## Reporting a problem

Report it privately through GitHub's [private vulnerability reporting][report].
Please don't open a public issue for it.

[report]: https://github.com/uvw/claudes/security/advisories/new

Include:

1. The claudes version (`claudes --version`) and the Claude Code version
   (`claude --version`).
2. Your operating system and shell.
3. What an attacker could do, and the steps to show it.

claudes stops and starts Claude Code sessions. It writes `pins.json`, its park
file, `config.yaml`, a cached Claude Code version, the agent skill, and the
transcript copies `session mv` makes, with its record of them. It deletes
finished background sessions (`gc`), team folders and task lists (`gc --teams`),
and the original transcript after `session mv --rm`. Reports about it acting on
the wrong session or process, about injection through the commands and names it
prints, or about those files are in scope.

## Checking a download

Every archive and SBOM in a release has a build provenance attestation:

```bash
gh attestation verify claudes_0.1.0_linux_amd64.tar.gz --repo uvw/claudes
```

`checksums.txt` in each release lists the SHA-256 of every archive and SBOM. It
is not attested itself, so check an archive's attestation rather than only its
checksum.
