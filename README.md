# Viceme CLI

`viceme` is the deterministic client used by the bundled Viceme Agent Skill. It authenticates, uploads local Skill bundles, creates and observes Skill Agent publications, and installs the matching Skill documentation into supported Agent environments. Source parsing, LLM compilation, BuildRun materialization, and share publication remain server-side.

## Development

```bash
make build
make test
make check
```

Set `VICEME_API_BASE_URL` for local integration. Production credentials are stored only in the operating system keychain; there is no plaintext token fallback.

The npm/Homebrew download shell, signed release manifest, and self-update implementation are intentionally outside the first Go MVP. `make update-check` reserves the non-mutating interface until those release controls exist.
