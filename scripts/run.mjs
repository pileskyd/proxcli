export async function run(cmd, cwd = process.cwd()) {
  const child = Bun.spawn(cmd, { cwd, stdin: 'inherit', stdout: 'inherit', stderr: 'inherit' })
  const code = await child.exited
  if (code !== 0) throw new Error(`${cmd[0]} завершился с кодом ${code}`)
}
