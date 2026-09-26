# Maintainer setup

One-time GitHub repo settings the maintainer must enable for the full
release pipeline to work end to end. None of this can be set via
checked-in code; it requires repo admin.

## 1. Homebrew tap auto-update

GoReleaser publishes a Homebrew formula update to
[`felixgeelhaar/homebrew-tap`](https://github.com/felixgeelhaar/homebrew-tap)
on each tagged release. This requires a personal access token (PAT)
with write access to that tap repo, exposed to the workflow as
`HOMEBREW_TAP_TOKEN`.

**Steps:**

1. Create a fine-grained PAT scoped to `felixgeelhaar/homebrew-tap`
   only, with `Contents: Read and write`. Do **not** scope it to all
   repos. Expiry: 1 year max.
2. Add it as a repo secret on `felixgeelhaar/roady`:
   `Settings → Secrets and variables → Actions → New repository secret`.
   Name: `HOMEBREW_TAP_TOKEN`. Value: the PAT.
3. Verify by re-running the release workflow on an existing tag, or
   by cutting a fresh patch tag.

When this secret is missing the release still publishes binaries, but
the goreleaser run reports a 401 on the homebrew step and `brew install
felixgeelhaar/tap/roady` lags behind.

## 2. GitHub Pages

The Astro website auto-deploys via
`.github/workflows/website.yml` (added in v0.11). Pages must be
enabled on the repo first.

**Steps:**

1. `Settings → Pages → Build and deployment → Source: GitHub Actions`.
2. (Optional) Custom domain: configure under the same page; remember
   to set the `CNAME` and update `website/astro.config.mjs` if you
   want absolute asset URLs.
3. Push any change under `website/`, `README.md`, or `docs/` to
   trigger the workflow. The first run cold-starts a few minutes;
   subsequent runs are <1 min.

## 3. Quality gate inventory

What's gated where, as of v0.10.1+:

| Tool | Pre-commit hook | CI |
| --- | --- | --- |
| `golangci-lint` (`govet`, `gocritic`, `errcheck`, `staticcheck`, `unused`) | yes | yes |
| `gofmt`, `goimports` (formatters in `golangci-lint` v2) | yes | yes |
| `coverctl` (domain thresholds from `.coverctl.yaml`) | yes | yes |
| `nox scan --staged` (security scanner) | yes (advisory) | **local-only** |

`nox` is a private binary, not publicly distributed via `go install` or
Homebrew. CI does not run it. Maintainers running the pre-commit hook
locally see findings before push; everyone else sees them only at
review time. If `nox` ever ships publicly, add a CI step in
`.github/workflows/ci.yml` that mirrors the hook (advisory, non-gating).

## 4. Telemetry

Roady collects no usage telemetry. Local usage tracking (`roady usage`,
`usage.json`) was removed.

## 5. Roady Cloud waitlist inbox

The hero form opens the visitor's mail client with a `mailto:` to
`roady-cloud@felixgeelhaar.de`. If you want a different inbox, update
`WAITLIST_INBOX` in `website/src/components/HeroSection.vue`.
Auto-responder is the maintainer's responsibility.

## 6. Pre-commit hook (developer side)

Optional but recommended for local contributors:

```bash
./scripts/setup-hooks.sh
```

Runs the same gates CI runs (build / lint / test+coverage / verdict /
nox) before every commit so contributors don't push obviously broken
work. Skips can be requested explicitly with `git commit --no-verify`.
