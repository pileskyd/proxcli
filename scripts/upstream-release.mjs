import { appendFile, readFile } from 'node:fs/promises'

const tagPattern = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

function versionParts(tag) {
  if (typeof tag !== 'string' || (!tagPattern.test(tag) || tag.trim() !== tag)) throw new Error('Invalid upstream release tag')
  return tag.slice(1).split('.').map(BigInt)
}

export function releaseStatus(pinned, release) {
  const current = versionParts(pinned)
  const latest = versionParts(release.tag_name)
  if (release.draft !== false || release.prerelease !== false) throw new Error('Expected a stable upstream release')
  let newer = false
  for (let i = 0; i < current.length; i++) {
    if (latest[i] === current[i]) continue
    newer = latest[i] > current[i]
    break
  }
  return { version: release.tag_name, newer }
}

export async function latestRelease(pinned, token, fetcher = fetch) {
  const headers = { Accept: 'application/vnd.github+json', 'User-Agent': 'proxcli-upstream-check', 'X-GitHub-Api-Version': '2022-11-28' }
  if (token) headers.Authorization = `Bearer ${token}`
  const response = await fetcher('https://api.github.com/repos/router-for-me/CLIProxyAPI/releases/latest', {
    headers,
    signal: AbortSignal.timeout(30_000),
  })
  if (!response.ok) throw new Error(`Upstream release lookup failed: HTTP ${response.status}`)
  return releaseStatus(pinned, await response.json())
}

if (import.meta.main) {
  const pinned = JSON.parse(await readFile(new URL('../upstream.json', import.meta.url), 'utf8'))
  const status = await latestRelease(pinned.version, process.env.GH_TOKEN)
  const url = `https://github.com/router-for-me/CLIProxyAPI/releases/tag/${status.version}`
  console.log(`Pinned: ${pinned.version}; latest: ${status.version}; newer: ${status.newer}`)
  if (process.env.GITHUB_OUTPUT) {
    await appendFile(process.env.GITHUB_OUTPUT, `version=${status.version}\nnewer=${status.newer}\n`)
  }
  if (process.env.GITHUB_STEP_SUMMARY) {
    await appendFile(process.env.GITHUB_STEP_SUMMARY, `Pinned CLIProxyAPI: **${pinned.version}** (${pinned.commit}). Latest stable release: [${status.version}](${url}).\n\n${status.newer ? 'A compatibility build will apply the current patch and run core, observer and panel contract tests. The pin stays unchanged.' : 'No newer stable release; compatibility probe skipped.'}\n`)
  }
}
