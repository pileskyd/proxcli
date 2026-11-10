import { mkdir } from 'node:fs/promises'
import { run } from './run.mjs'
await run(['bun', 'install', '--frozen-lockfile'], `${process.cwd()}/web`)
await run(['bun', 'run', 'build'], `${process.cwd()}/web`)
const panelBinary = process.platform === 'win32' ? 'bin/proxcli.exe' : 'bin/proxcli'
await mkdir('bin', { recursive: true })
await run(['go', 'build', '-trimpath', '-o', panelBinary, './cmd/proxcli'])
