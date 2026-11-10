import { test, expect } from 'bun:test'
import { createDraft, resolveDraft, parseProxyUrl, effectiveExit, applyPastedUrl } from '../src/lib/proxy.ts'

test('masked password is never sent back as a real password', () => {
 const d = createDraft('custom', 'socks5h://u:***@proxy:1080')
 expect(resolveDraft(d)).toEqual({ ok: true, proxyMode: 'custom', changed: false })
 d.host = 'another-proxy'
 expect(resolveDraft(d).ok).toBe(false)
 d.replacePassword = true
 d.password = 'new@pass'
 expect(resolveDraft(d).proxyUrl).toBe('socks5h://u:new%40pass@another-proxy:1080')
})
test('IPv6 and escaped password roundtrip', () => {
 const d = createDraft('custom', '')
 expect(applyPastedUrl(d, 'socks5h://name:p%40ss%3Aword@[::1]:1080')).toBe(true)
 expect(parseProxyUrl(resolveDraft(d).proxyUrl).password).toBe('p@ss:word')
})
test('invalid ports rejected and imported account not claimed direct', () => {
 const d = createDraft('custom', ''); d.host = 'proxy';d.port='65536'
 expect(resolveDraft(d).ok).toBe(false)
 expect(effectiveExit({proxyMode:'inherit',proxyUrl:'',proxyKnown:false},'').kind).toBe('unknown')
 expect(effectiveExit({proxyMode:'inherit',proxyUrl:'',proxyKnown:true},'direct').kind).toBe('direct')
})
