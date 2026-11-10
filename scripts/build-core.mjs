import { mkdtemp, mkdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { run } from './run.mjs'

// The patch and its tests apply only to this exact upstream commit.
const commit = '6db7d4b839d86c5b24e17bbe73bcd4db18ef1b1a'
const root = process.cwd()
const exe = process.platform === 'win32' ? '.exe' : ''
const temporary = await mkdtemp(join(tmpdir(), 'proxcli-core-build-'))
try {
  const source = join(temporary, 'upstream')
  await run(['git', 'clone', '--depth', '1', '--branch', 'v8.0.63', 'https://github.com/router-for-me/CLIProxyAPI.git', source])
  const rev = Bun.spawn(['git', 'rev-parse', 'HEAD'], { cwd: source, stdout: 'pipe' })
  const actual = (await new Response(rev.stdout).text()).trim()
  if (await rev.exited !== 0 || actual !== commit) throw new Error('Upstream tag does not match the pinned commit')
  await run(['git', 'apply', '--check', resolve('patches/cliproxyapi-v8.0.63-quota-strict.patch')], source)
  await run(['git', 'apply', resolve('patches/cliproxyapi-v8.0.63-quota-strict.patch')], source)
  await run(['go', 'test', './sdk/cliproxy/auth', '-count=1'], source)
  await run(['go', 'test', './sdk/cliproxy', '-run', 'TestServiceApplyConfigRuntimeSessionAffinityRebindOnChangeRecreatesSelector', '-count=1'], source)
  await run(['go', 'test', './internal/pluginhost', '-run', 'Test(Unix|Windows|LimitedProbe)', '-count=1'], source)
  await mkdir(join(root, 'bin/plugins'), { recursive: true })
  await run(['go', 'build', '-trimpath', '-o', join(root, `bin/cli-proxy-api${exe}`), './cmd/server'], source)
  const extension = process.platform === 'darwin' ? 'dylib' : process.platform === 'win32' ? 'dll' : 'so'
  await run(['go', 'build', '-trimpath', '-buildmode=c-shared', '-o', `bin/plugins/proxcli-usage.${extension}`, './plugins/usage'])
  console.log('CLIProxyAPI v8.0.63 + quota-strict и usage observer собраны в bin/.')
} finally {
  // This directory is created solely by this command; it contains no user checkout.
  await rm(temporary, { recursive: true, force: true })
}
