# Third-party components

ProxCli is MIT licensed. Bundled dependencies retain their original licenses.

- CLIProxyAPI v8.0.63, commit `6db7d4b839d86c5b24e17bbe73bcd4db18ef1b1a`, by Router-For.ME and contributors: MIT. The release contains a patched binary; the exact patch is in `patches/cliproxyapi-v8.0.63-quota-strict.patch`. The C ABI adapter follows the upstream plugin examples. License: `licenses/CLIProxyAPI-MIT.txt`.
- Golos Text, distributed through Fontsource: SIL Open Font License, bundled in `licenses/Golos-Text-OFL.txt`.
- PT Mono, distributed through Fontsource: SIL Open Font License, bundled in `licenses/PT-Mono-OFL.txt`.
- Vue (MIT; `licenses/Vue-MIT.txt`) and its frontend dependencies are bundled by Vite; their package licenses apply. Dependency versions are recorded in `web/bun.lock`.
- CLIProxyAPI's Go dependencies remain governed by their own licenses. The pinned upstream `go.mod` and `go.sum` and `go version -m bin/cli-proxy-api` identify its dependencies.

This project is independent of Router-For.ME and Theo / T3. Links to their work describe the technical base and inspiration, not endorsement.
