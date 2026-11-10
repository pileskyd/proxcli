import { run } from './run.mjs'
await run(['bun', 'install', '--frozen-lockfile'], `${process.cwd()}/web`)
await run(['go', 'test', '-race', './...'])
await run(['go', 'vet', './...'])
await run(['bun', 'test', './tests'], `${process.cwd()}/web`)
await run(['bun', 'run', 'build'], `${process.cwd()}/web`)
