import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { parseArgs } from 'node:util'
import { run } from './run.mjs'

const pinned = JSON.parse(await readFile(new URL('../upstream.json', import.meta.url), 'utf8'))
const { values } = parseArgs({ options: { upstream: { type: 'string' }, 'output-dir': { type: 'string' } } })
// An explicit override probes compatibility; production builds always verify the pin.
const version = values.upstream ?? pinned.version
if ((!/^v\d+\.\d+\.\d+$/.test(version) || version.trim() !== version)) throw new Error('Invalid upstream release tag')
const commit = values.upstream ? null : pinned.commit
const output = resolve(values['output-dir'] ?? 'bin')
// SDK tests must never open an operator's real affinity store from .env.
delete process.env.PROXCLI_AFFINITY_FILE
const exe = process.platform === 'win32' ? '.exe' : ''
const temporary = await mkdtemp(join(tmpdir(), 'proxcli-core-build-'))
try {
  const source = join(temporary, 'upstream')
  await run(['git', 'clone', '--depth', '1', '--branch', version, 'https://github.com/router-for-me/CLIProxyAPI.git', source])
  const rev = Bun.spawn(['git', 'rev-parse', 'HEAD'], { cwd: source, stdout: 'pipe' })
  const actual = (await new Response(rev.stdout).text()).trim()
  if (await rev.exited !== 0 || (commit && actual !== commit)) throw new Error('Upstream tag does not match the pinned commit')
  console.log(`CLIProxyAPI ${version} (${actual})${commit ? '' : ' — compatibility probe'}`)
  await run(['git', 'apply', '--check', resolve(pinned.patch)], source)
  await run(['git', 'apply', resolve(pinned.patch)], source)
  await run(['go', 'test', './sdk/cliproxy/auth', './sdk/cliproxy/session', '-count=1'], source)
  await run(['go', 'test', './sdk/cliproxy', '-run', 'TestServiceApplyConfigRuntimeSessionAffinityRebindOnChangeRecreatesSelector|TestProxCliDurableStateConfigured', '-count=1'], source)
  // Generic Go c-shared fixture plugins do not use ProxCli's Intel TLS isolation.
  // Test the supported observer path there; retain the full native-loader suite elsewhere.
  const loaderTests = process.platform === 'darwin' && process.arch === 'x64'
    ? 'Test(ProxCliUsageNativeThreadLifecycle|LimitedProbe)'
    : 'Test(Unix|Windows|DynamicLibrary|LimitedProbe|ProxCliUsageNativeThreadLifecycle)'
  await run(['go', 'test', './internal/pluginhost', '-run', loaderTests, '-count=1'], source)
  await mkdir(join(output, 'plugins'), { recursive: true })
  await run(['go', 'build', '-trimpath', '-o', join(output, `cli-proxy-api${exe}`), './cmd/server'], source)
  const extension = process.platform === 'darwin' ? 'dylib' : process.platform === 'win32' ? 'dll' : 'so'
  await run(['go', 'build', '-trimpath', '-buildmode=c-shared', '-o', join(output, `plugins/proxcli-usage.${extension}`), './plugins/usage'])
  console.log(`CLIProxyAPI ${version} + quota-strict и usage observer собраны в ${output}.`)
} finally {
  // This directory is created solely by this command; it contains no user checkout.
  await rm(temporary, { recursive: true, force: true })
}
