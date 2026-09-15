# Third-Party Licenses — huddle

`huddle` bundles **no third-party code**.

- **Go binary.** `go.mod` declares no `require` directives and the repository has
  no `go.sum`; every import resolves to the Go standard library. The embedded
  dashboard (`web/`) is hand-written HTML, CSS and JavaScript with no framework,
  no CDN and no build step, so nothing is vendored there either.
- **npm wrapper.** `npm/package.json` has no runtime `dependencies`. Its
  `optionalDependencies` are huddle's own per-platform binary packages
  (`@muthuishere/huddle-<os>-<arch>`), published from this repository.

There is therefore nothing to attribute here. huddle itself is MIT — see `LICENSE`.

The Go toolchain and standard library are distributed by the Go authors under a
BSD-3-Clause license; that covers the compiler and the packages linked into the
release binaries, and its terms travel with the Go distribution rather than with
this source tree.

If a third-party dependency is ever added, regenerate this file before
publishing a release — `go list -deps ./...` for the binary and
`npm ls --omit=dev` for the wrapper.
