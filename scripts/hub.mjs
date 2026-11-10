import { resolve } from 'node:path'
import { access } from 'node:fs/promises'
const coreBinary = process.platform === 'win32' ? './bin/cli-proxy-api.exe' : './bin/cli-proxy-api'
try { await access(coreBinary) } catch { throw new Error('Сначала выполните bun run build:core. Для quota-strict нужен собранный ProxCli core.') }
if (!process.env.PROXCLI_AFFINITY_FILE) process.env.PROXCLI_AFFINITY_FILE = resolve(process.env.PROXCLI_DATA_DIR ?? '.data', 'affinity.enc')
const child = Bun.spawn([coreBinary, '-config', '.data/cliproxyapi.yaml'], { stdin: 'inherit', stdout: 'inherit', stderr: 'inherit' })
process.on('SIGINT', () => child.kill('SIGINT'))
process.on('SIGTERM', () => child.kill('SIGTERM'))
process.exitCode = await child.exited
