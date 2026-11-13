<script setup lang="ts">
import type Hls from 'hls.js'
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Play, X } from 'lucide-vue-next'
import { api } from '../api/client'
import { jableURL } from './jable'

const props = defineProps<{ id: string }>()
interface Playback { source: string; direct_url: string; relay_url: string; source_url: string }
interface TorrentPlayback { token: string; stream_url: string; source_url: string; seeders: number; title: string }
const video = ref<HTMLVideoElement | null>(null)
const open = ref(false)
const loading = ref(false)
const unavailable = ref(false)
const failed = ref(false)
const verificationRequired = ref(false)
const usingRelay = ref(false)
const current = ref<Playback | null>(null)
const torrent = ref<TorrentPlayback | null>(null)
const torrentMode = ref(false)
const buffering = ref(false)
let bufferTimer: ReturnType<typeof setTimeout> | undefined
let pending: AbortController | null = null
let player: Hls | undefined
let nativePlayback = false
let requestVersion = 0

function destroy() {
  clearTimeout(bufferTimer); buffering.value = false
  nativePlayback = false
  player?.destroy(); player = undefined
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
}
function onWaiting() {
  if (!torrentMode.value || !torrent.value || failed.value) return
  buffering.value = true
  clearTimeout(bufferTimer)
  bufferTimer = setTimeout(() => { if (torrentMode.value && buffering.value) { buffering.value = false; failed.value = true } }, 75_000)
}
function onPlayable() { clearTimeout(bufferTimer); buffering.value = false }
async function playStream(url: string) {
  if (!video.value) return
  destroy()
  const { default: Hls } = await import('hls.js')
  if (!current.value || !video.value) return
  if (Hls.isSupported()) {
    player = new Hls()
    player.on(Hls.Events.ERROR, (_event, data) => { if (data.fatal) fallback() })
    player.loadSource(url); player.attachMedia(video.value)
  } else if (video.value.canPlayType('application/vnd.apple.mpegurl')) {
    nativePlayback = true
    video.value.src = url
  } else { failed.value = true }
}
function fallback() {
  if (!current.value) return
  if (!usingRelay.value) { usingRelay.value = true; void playStream(current.value.relay_url) } else { destroy(); failed.value = true }
}
function onVideoError() { if (nativePlayback) fallback() }
async function start() {
  stopTorrent()
  torrentMode.value = false
  const version = ++requestVersion
  current.value = null
  destroy()
  open.value = true; loading.value = true; unavailable.value = false; failed.value = false; verificationRequired.value = false; usingRelay.value = false
  try {
    const sources = (await api<Playback[]>(`/movies/${encodeURIComponent(props.id)}/playbacks`)).data
    if (version !== requestVersion) return
    current.value = sources[0] || null
    if (!current.value) unavailable.value = true
    else { await nextTick(); await playStream(current.value.direct_url) }
  } catch (error) {
    if (version === requestVersion) {
      verificationRequired.value = typeof error === 'object' && error !== null && 'code' in error && error.code === 'playback_verification_required'
      failed.value = true
    }
  } finally { if (version === requestVersion) loading.value = false }
}
function stopTorrent() {
  pending?.abort(); pending = null
  if (torrent.value) {
    const token = torrent.value.token
    torrent.value = null
    void api(`/torrent-sessions/${encodeURIComponent(token)}`, { method: 'DELETE', keepalive: true }).catch(() => {})
  }
}
async function startTorrent() {
  stopTorrent()
  const version = ++requestVersion
  current.value = null; destroy(); torrentMode.value = true
  open.value = true; loading.value = true; unavailable.value = false; failed.value = false; verificationRequired.value = false
  const controller = new AbortController(); pending = controller
  try {
    const res = await api<TorrentPlayback>(`/movies/${encodeURIComponent(props.id)}/torrent-sessions`, { method: 'POST', signal: controller.signal })
    if (version !== requestVersion) {
      void api(`/torrent-sessions/${encodeURIComponent(res.data.token)}`, { method: 'DELETE', keepalive: true }).catch(() => {})
      return
    }
    torrent.value = res.data
    await nextTick()
    if (version === requestVersion && video.value) {
      onWaiting()
      video.value.src = res.data.stream_url
      void video.value.play().catch(() => {})
    }
  } catch { if (version === requestVersion) failed.value = true } finally { if (version === requestVersion) { loading.value = false; pending = null } }
}
function close() { requestVersion++; stopTorrent(); current.value = null; destroy(); open.value = false }
watch(() => props.id, close)
onBeforeUnmount(close)
</script>

<template>
  <section class="detail-section playback-section">
    <div class="admin-heading"><h3>{{ $t('player.title') }}</h3><button v-if="!open" class="btn search-button btn-sm" @click="start"><Play :size="15" />{{ $t('player.play') }}</button><button v-else class="btn btn-ghost btn-square btn-sm" :aria-label="$t('common.close')" @click="close"><X :size="17" /></button></div>
    <a v-if="jableURL(id)" class="btn btn-outline btn-sm mt-2" :href="jableURL(id)" target="_blank" rel="noopener noreferrer">{{ $t('player.openJable') }}</a>
    <button v-if="!open || !torrentMode" data-testid="torrent-play" class="btn btn-outline btn-sm mt-2 ml-2" @click="startTorrent">{{ $t('player.torrentPlay') }}</button>
    <template v-if="open"><div class="video-frame"><video ref="video" controls playsinline preload="metadata" :aria-label="$t('player.title')" @waiting="onWaiting" @playing="onPlayable" @canplay="onPlayable" @error="torrentMode ? (buffering = false, failed = true) : onVideoError()"></video><div v-if="loading || buffering || unavailable || failed" class="video-state" :class="{ 'video-buffering': buffering && !failed }"><span v-if="loading || (buffering && !failed)" class="loading loading-spinner loading-sm"></span><span v-if="loading && torrentMode">{{ $t('player.connecting') }}</span><span v-else-if="buffering && !failed">{{ $t('player.buffering') }}</span><span v-else-if="!loading">{{ $t(torrentMode ? 'player.torrentFailed' : verificationRequired ? 'player.verificationRequired' : unavailable ? 'player.unavailable' : 'player.failed') }}</span></div></div><div v-if="current" class="playback-info"><span>{{ current.source.toUpperCase() }}</span><span v-if="usingRelay">{{ $t('player.relay') }}</span><a :href="current.source_url" target="_blank" rel="noopener noreferrer">{{ $t('movies.visit') }}</a></div><div v-if="torrent" class="playback-info"><span>{{ $t('player.seeders', { count: torrent.seeders }) }}</span><a :href="torrent.source_url" target="_blank" rel="noopener noreferrer">Sukebei</a></div><button v-if="unavailable || failed" class="btn btn-outline btn-sm" @click="torrentMode ? startTorrent() : start()">{{ $t('common.retry') }}</button></template>
  </section>
</template>
