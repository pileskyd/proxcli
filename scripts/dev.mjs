import { mkdir } from 'node:fs/promises'
import { run } from './run.mjs'
await run(['bun', 'install', '--frozen-lockfile'], `${process.cwd()}/web`)
const panelBinary = process.platform === 'win32' ? 'bin/proxcli.exe' : 'bin/proxcli'
await mkdir('bin', { recursive: true })
await run(['go', 'build', '-o', panelBinary, './cmd/proxcli'])
const opts = { stdin: 'inherit', stdout: 'inherit', stderr: 'inherit' }
const server = Bun.spawn([`./${panelBinary}`], opts)
const web = Bun.spawn(['bun', 'run', 'dev'], { ...opts, cwd: `${process.cwd()}/web` })
const stop = () => { server.kill('SIGTERM'); web.kill('SIGTERM') }
process.on('SIGINT', stop)
process.on('SIGTERM', stop)
await Promise.race([server.exited, web.exited])
stop()
await Promise.all([server.exited, web.exited])
