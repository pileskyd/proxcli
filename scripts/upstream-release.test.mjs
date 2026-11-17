import { expect, test } from 'bun:test'
import { latestRelease, releaseStatus } from './upstream-release.mjs'

const release = (tag_name, extra = {}) => ({ tag_name, draft: false, prerelease: false, ...extra })

test('compares release versions numerically across patch, minor and major boundaries', () => {
  for (const [pinned, tag, newer] of [
    ['v8.0.76', 'v8.0.76', false],
    ['v8.0.76', 'v8.0.75', false],
    ['v8.0.76', 'v8.0.100', true],
    ['v8.9.99', 'v8.10.0', true],
    ['v8.99.99', 'v9.0.0', true],
    ['v9.0.0', 'v8.99.99', false],
  ]) expect(releaseStatus(pinned, release(tag))).toEqual({ version: tag, newer })
})

test('rejects unstable releases and tags that cannot safely enter workflow outputs', () => {
  for (const tag of ['v8.0.77\nnewer=true', 'v8.0.77\n', '--upload-pack=evil', 'v8.0.77-rc.1', 'v08.0.77', null]) {
    expect(() => releaseStatus('v8.0.76', release(tag))).toThrow()
  }
  expect(() => releaseStatus('v8.0.76', release('v8.0.77', { draft: true }))).toThrow()
  expect(() => releaseStatus('v8.0.76', release('v8.0.77', { prerelease: true }))).toThrow()
})

test('API failures fail the check instead of reporting up to date', async () => {
  for (const status of [403, 404, 429, 500]) {
    await expect(latestRelease('v8.0.76', undefined, async () => new Response('', { status }))).rejects.toThrow(`HTTP ${status}`)
  }
  await expect(latestRelease('v8.0.76', undefined, async () => { throw new Error('offline') })).rejects.toThrow('offline')
})

test('looks up latest stable release through the fixed upstream API', async () => {
  const status = await latestRelease('v8.0.76', 'test-token', async (url, options) => {
    expect(url).toBe('https://api.github.com/repos/router-for-me/CLIProxyAPI/releases/latest')
    expect(options.headers.Authorization).toBe('Bearer test-token')
    return Response.json(release('v8.0.77'))
  })
  expect(status).toEqual({ version: 'v8.0.77', newer: true })
})
