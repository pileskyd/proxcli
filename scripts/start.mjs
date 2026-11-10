const binary = process.platform === 'win32' ? './bin/proxcli.exe' : './bin/proxcli'
const child = Bun.spawn([binary, 'serve'], { stdin: 'inherit', stdout: 'inherit', stderr: 'inherit' })
process.on('SIGINT', () => child.kill('SIGTERM'))
process.on('SIGTERM', () => child.kill('SIGTERM'))
process.exitCode = await child.exited
