import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WikiPresenceBar from './WikiPresenceBar.vue'

describe('WikiPresenceBar', () => {
  it('shows "Solo" when no peers', () => {
    const w = mount(WikiPresenceBar, { props: { peers: [], connected: false } })
    expect(w.text()).toContain('Solo')
  })

  it('renders avatars for peers', () => {
    const peers = [
      { clientId: 1, displayName: 'Alice Wong', color: '#58a6ff' },
      { clientId: 2, displayName: 'Bob', color: '#f85149' },
    ]
    const w = mount(WikiPresenceBar, { props: { peers, connected: true } })
    expect(w.findAll('.wiki-presence-avatar')).toHaveLength(2)
    expect(w.text()).toContain('AW')
    expect(w.text()).toContain('B')
  })

  it('caps visible peers and shows overflow', () => {
    const peers = Array.from({ length: 8 }, (_, i) => ({
      clientId: i + 1, displayName: `User${i}`, color: '#58a6ff',
    }))
    const w = mount(WikiPresenceBar, { props: { peers, connected: true, max: 5 } })
    expect(w.findAll('.wiki-presence-avatar')).toHaveLength(5)
    expect(w.text()).toContain('+3')
  })

  it('connection dot reflects state', () => {
    const w1 = mount(WikiPresenceBar, { props: { peers: [], connected: true } })
    expect(w1.find('.wiki-presence-dot').classes()).toContain('is-connected')

    const w2 = mount(WikiPresenceBar, { props: { peers: [], connected: false } })
    expect(w2.find('.wiki-presence-dot').classes()).toContain('is-connecting')

    const w3 = mount(WikiPresenceBar, { props: { peers: [], connected: false, error: 'boom' } })
    expect(w3.find('.wiki-presence-dot').classes()).toContain('is-error')
  })

  it('initials handles single-word names', () => {
    const peers = [{ clientId: 1, displayName: 'Madonna', color: '#000' }]
    const w = mount(WikiPresenceBar, { props: { peers, connected: true } })
    expect(w.text()).toContain('MA')
  })
})
