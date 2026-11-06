<script setup lang="ts">
import type Hls from 'hls.js'
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Play, X } from 'lucide-vue-next'
import { api } from '../api/client'

const props = defineProps<{ id: string }>()
interface Playback { source: string; direct_url: string; relay_url: string; source_url: string }
const video = ref<HTMLVideoElement | null>(null)
const open = ref(false)
const loading = ref(false)
const unavailable = ref(false)
const failed = ref(false)
const usingRelay = ref(false)
const current = ref<Playback | null>(null)
let player: Hls | undefined
let nativePlayback = false

function destroy() {
  nativePlayback = false
  player?.destroy(); player = undefined
  if (video.value) { video.value.pause(); video.value.removeAttribute('src'); video.value.load() }
}
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
  open.value = true; loading.value = true; unavailable.value = false; failed.value = false; usingRelay.value = false
  try {
    const sources = (await api<Playback[]>(`/movies/${encodeURIComponent(props.id)}/playbacks`)).data
    current.value = sources[0] || null
    if (!current.value) unavailable.value = true
    else { await nextTick(); await playStream(current.value.direct_url) }
  } catch { failed.value = true } finally { loading.value = false }
}
function close() { current.value = null; destroy(); open.value = false }
watch(() => props.id, close)
onBeforeUnmount(destroy)
</script>

<template>
  <section class="detail-section playback-section">
    <div class="admin-heading"><h3>{{ $t('player.title') }}</h3><button v-if="!open" class="btn search-button btn-sm" @click="start"><Play :size="15" />{{ $t('player.play') }}</button><button v-else class="btn btn-ghost btn-square btn-sm" :aria-label="$t('common.close')" @click="close"><X :size="17" /></button></div>
    <template v-if="open"><div class="video-frame"><video ref="video" controls playsinline preload="metadata" :aria-label="$t('player.title')" @error="onVideoError"></video><div v-if="loading || unavailable || failed" class="video-state"><span v-if="loading" class="loading loading-spinner loading-sm"></span><span v-else>{{ $t(unavailable ? 'player.unavailable' : 'player.failed') }}</span></div></div><div v-if="current" class="playback-info"><span>{{ current.source.toUpperCase() }}</span><span v-if="usingRelay">{{ $t('player.relay') }}</span><a :href="current.source_url" target="_blank" rel="noopener noreferrer">{{ $t('movies.visit') }}</a></div><button v-if="unavailable || failed" class="btn btn-outline btn-sm" @click="start">{{ $t('common.retry') }}</button></template>
  </section>
</template>
