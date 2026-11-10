import { access, cp, mkdir, mkdtemp, rm } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { run } from './run.mjs'
const version = process.env.PROXCLI_VERSION ?? 'v0.3.0'
if (!/^v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?$/.test(version)) throw new Error('Invalid release version')
const os = { darwin: 'macos', linux: 'linux', win32: 'windows' }[process.platform]
const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch]
if (!os || !arch) throw new Error('Unsupported release platform')
const extension = process.platform === 'darwin' ? 'dylib' : process.platform === 'win32' ? 'dll' : 'so'
const exe = process.platform === 'win32' ? '.exe' : ''
await mkdir('.release-stage', { recursive: true })
const stage = await mkdtemp(resolve('.release-stage', `proxcli-${version}-${os}-${arch}-`))
const output = resolve('dist-release')
await mkdir('bin', { recursive: true })
await run(['go', 'build', '-trimpath', '-ldflags', `-X main.version=${version}`, '-o', `bin/proxcli${exe}`, './cmd/proxcli'])
await access(`bin/cli-proxy-api${exe}`)
await access(`bin/plugins/proxcli-usage.${extension}`)
await access('web/dist/index.html')
await mkdir(stage, { recursive: true })
try {
  // Release allowlist. User data, environment files, logs and build checkouts are never copied.
  for (const path of [`bin/proxcli${exe}`, `bin/cli-proxy-api${exe}`, `bin/plugins/proxcli-usage.${extension}`, 'web/dist', 'web/bun.lock', 'configs', '.env.example', 'patches', 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'licenses', 'docs']) {
    const target = join(stage, path)
    await mkdir(join(target, '..'), { recursive: true })
    await cp(path, target, { recursive: true })
  }
  await run([process.platform === 'win32' ? 'python' : 'python3', 'scripts/archive-release.py', stage, output, `proxcli-${version}-${os}-${arch}`])
} finally {
  await rm(stage, { recursive: true, force: true })
}
