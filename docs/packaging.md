# Packaging and install channels

One `v*` tag drives every channel. `.goreleaser.yaml` is the single source of
truth; `.github/workflows/npm-publish.yml` runs it.

| Channel | Artifact | Needs | Status |
|---|---|---|---|
| npm | `@muthuishere/huddle` + 6 platform packages | npm trusted publisher, per package | **not bootstrapped** |
| Homebrew | cask in `muthuishere/homebrew-tap` | `HOMEBREW_TAP_GITHUB_TOKEN` | ready, token not set |
| apt / dnf | `.deb` + `.rpm` on the GitHub Release | nothing | **ready** |
| winget | PR to `microsoft/winget-pkgs` | `WINGET_GITHUB_TOKEN`, a fork, public repo | ready, token not set |
| GitHub Release | tar.gz, zip, `checksums.txt` | `GITHUB_TOKEN` (built in) | **ready** |

**Installing huddle installs the skills.** The skill family is the product, so
no channel leaves a half-finished install. npm does it in `postinstall` and the
brew cask in `postflight`, but the guarantee does not rest on those: `huddle
start` syncs the embedded skills whenever they are absent or stale, which also
covers apt, dnf, winget, `go install` and a downloaded tarball. Hooks could
never have covered those — apt and dnf run as root, so there is no user
`~/.claude` to populate, and winget portables have no post-install hook at all.

Sync writes only where a skill is missing, where huddle owns a stale copy, or
where the old `install.sh` symlink into a skill repo is still in place — that
last case is the upgrade. It removes a link, never the repo behind it, and never
state: memories, personas and the event log live under the config roots, which
are copied, never overwritten (ADR 6). A dir huddle does not own is reported and
left alone. Opt out with `HUDDLE_NO_SKILL_SYNC=1` or `"skillSync": false`.

Every optional channel is guarded by `skip_upload`, so a missing token writes
the artifact to `dist/` and skips the push rather than failing the release.
That is what makes `goreleaser release --snapshot --clean --skip=publish`
runnable on a laptop.

**Six build targets, not five.** darwin and linux on `amd64` + `arm64`, windows on
`amd64` + `arm64`. windows/arm64 is built because npm's `os` x `cpu` fields are a
cross product: a launcher that claims `win32` and `arm64` is installable on
`win32-arm64` no matter what we intend, so the only honest choice is to ship a
binary that runs there. `CGO_ENABLED=0` means the target costs one build.

Adding a target means three places, all kept in lockstep by CI: the goreleaser
`goarch` list, `npm/platforms/<os>-<arch>/package.json`, and the `SUPPORTED` map
in `npm/bin/huddle.js`. `scripts/npm-publish-oidc.sh` also carries a literal
platform list — a target missing from it is built, archived and never published.

## What each channel gives the user

```bash
# Homebrew (macOS and Linux, Intel and ARM)
brew install --cask muthuishere/tap/huddle

# Debian / Ubuntu — from the release asset
curl -LO https://github.com/muthuishere/huddle-next/releases/latest/download/huddle_<version>_linux_amd64.deb
sudo apt install ./huddle_<version>_linux_amd64.deb

# Fedora / RHEL
sudo dnf install ./huddle_<version>_linux_amd64.rpm

# Windows
winget install Muthuishere.Huddle

# npm, any platform
npm i -g @muthuishere/huddle
```

No `huddle install --skills` step: every one of these leaves the skills
installed. Run it by hand only to force a reinstall, or with `--force` to take
over a skill dir huddle does not own.

## The one real gap: `apt-get install huddle`

The table above ships a `.deb`, which `apt install ./file.deb` accepts. A bare
`apt-get install huddle` is different: it needs an APT **repository** — a
`Packages` index and a `Release` file signed with a GPG key, served over HTTPS,
with the key distributed to users. GoReleaser builds packages; it does not host
a repository.

Three ways to close it, cheapest first:

1. **Leave it.** `apt install ./*.deb` covers CI and one-off installs, and most
   users of a dev CLI arrive via brew, winget or npm. No infrastructure.
2. **Hosted** — Gemfury, Cloudsmith or packagecloud give an apt + yum repo and
   a `publishers:` block in `.goreleaser.yaml` pushes to it. Fastest path to a
   real `apt-get install`; costs money per month and adds a vendor.
3. **Self-hosted on Cloudflare R2** — `aptly` or `reprepro` generates a signed
   repo, upload it to R2, serve it behind a domain. No vendor and no recurring
   fee beyond storage, but the signing key becomes a secret to protect and
   rotate, and a broken index breaks installs silently.

Pick 1 until there is a user asking for 2 or 3.

## One-time setup before the first release

Ordered, because several steps depend on the repo being public.

1. **Make the repo public.** winget validation and npm `--provenance` both
   require a public source repo. Nothing below works until this is done.
2. **Bootstrap the seven npm packages.** Trusted publishing is configured per
   package in npm's web UI, which cannot be done for a package that does not
   exist. Publish each once from a granular token, then set Trusted Publisher →
   GitHub Actions → `muthuishere/huddle-next` → `npm-publish.yml` on all seven:
   the `@muthuishere/huddle` launcher and `@muthuishere/huddle-{darwin,linux}-{arm64,x64}`
   plus `@muthuishere/huddle-windows-{arm64,x64}`.
3. **Create `muthuishere/homebrew-tap`** if absent (it exists, holding
   `Casks/crossmem.rb`) and add `HOMEBREW_TAP_GITHUB_TOKEN` — a fine-grained
   token with `contents: write` on that repo only.
4. **Fork `microsoft/winget-pkgs`** to `muthuishere/winget-pkgs` and add
   `WINGET_GITHUB_TOKEN` with `contents: write` and `pull_requests: write` on
   the fork. The first submission is reviewed by Microsoft's bot and can take
   a few days; later versions are usually automatic.
5. **Tag.** `scripts/npm-set-version.sh <version>`, commit, then
   `git tag v<version> && git push --tags`.

## Verifying a release actually worked

Check the artifact, not the green tick:

```bash
gh release view v<version>                      # 6 archives + checksums + deb/rpm
npm view @muthuishere/huddle version            # all 7 packages live
brew install --cask muthuishere/tap/huddle      # on a clean machine
winget install Muthuishere.Huddle               # on Windows
huddle doctor                                   # exits 0, every check OK
```
