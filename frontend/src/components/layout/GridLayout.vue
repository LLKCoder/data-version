<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { Panel } from '@/types/dashboard'

const props = withDefaults(defineProps<{ panels: Panel[]; editable?: boolean }>(), { editable: false })
const emit = defineEmits<{ update: [panels: Panel[]] }>()

const shell = ref<HTMLElement | null>(null)
const width = ref(0)
const rowHeight = 72
const gap = 16
const mobile = computed(() => width.value > 0 && width.value < 720)
const active = ref<{ kind: 'drag' | 'resize'; uid: string; startX: number; startY: number; origin: Panel } | null>(null)

const height = computed(() => {
  if (mobile.value) return Math.max(props.panels.length * 300, 300)
  const lastRow = Math.max(...props.panels.map((panel) => (panel.y || 0) + Math.max(panel.h || 4, 3)), 4)
  return lastRow * rowHeight + Math.max(lastRow - 1, 0) * gap
})

function panelStyle(panel: Panel, index: number) {
  if (mobile.value) {
    return { left: '0px', top: `${index * 300}px`, width: '100%', height: '284px' }
  }
  const columns = 12
  const columnWidth = Math.max((width.value - gap * (columns - 1)) / columns, 1)
  const columnSize = columnWidth + gap
  const w = Math.max(panel.w || 6, 2)
  const h = Math.max(panel.h || 4, 3)
  return {
    left: `${Math.max(panel.x || 0, 0) * columnSize}px`,
    top: `${Math.max(panel.y || 0, 0) * (rowHeight + gap)}px`,
    width: `${w * columnWidth + (w - 1) * gap}px`,
    height: `${h * rowHeight + (h - 1) * gap}px`,
  }
}

function startInteraction(event: PointerEvent, panel: Panel, kind: 'drag' | 'resize') {
  if (!props.editable || mobile.value) return
  active.value = { kind, uid: panel.uid, startX: event.clientX, startY: event.clientY, origin: { ...panel } }
  window.addEventListener('pointermove', moveInteraction)
  window.addEventListener('pointerup', stopInteraction, { once: true })
}

function moveInteraction(event: PointerEvent) {
  if (!active.value || !width.value) return
  const item = active.value
  const columnWidth = Math.max((width.value - gap * 11) / 12, 1)
  const columnSize = columnWidth + gap
  const dx = Math.round((event.clientX - item.startX) / columnSize)
  const dy = Math.round((event.clientY - item.startY) / (rowHeight + gap))
  const next = props.panels.map((panel) => ({ ...panel }))
  const target = next.find((panel) => panel.uid === item.uid)
  if (!target) return
  if (item.kind === 'drag') {
    target.x = Math.max(0, Math.min(12 - Math.max(item.origin.w || 6, 2), (item.origin.x || 0) + dx))
    target.y = Math.max(0, (item.origin.y || 0) + dy)
  } else {
    target.w = Math.max(2, Math.min(12 - (item.origin.x || 0), (item.origin.w || 6) + dx))
    target.h = Math.max(3, Math.min(12, (item.origin.h || 4) + dy))
  }
  emit('update', next)
}

function stopInteraction() {
  active.value = null
  window.removeEventListener('pointermove', moveInteraction)
}

function updateWidth() {
  width.value = shell.value?.clientWidth ?? 0
}

let observer: ResizeObserver | undefined
onMounted(() => {
  updateWidth()
  if (shell.value) {
    observer = new ResizeObserver(updateWidth)
    observer.observe(shell.value)
  }
})
onUnmounted(() => {
  observer?.disconnect()
  stopInteraction()
})
</script>

<template>
  <div ref="shell" class="grid-shell relative w-full" :style="{ height: `${height}px` }">
    <div
      v-for="(panel, index) in panels"
      :key="panel.uid"
      class="absolute min-w-0 transition-[left,top,width,height] duration-100"
      :class="{ 'z-10': active?.uid === panel.uid, 'cursor-move': editable && !mobile }"
      :style="panelStyle(panel, index)"
      @pointerdown="startInteraction($event, panel, 'drag')"
    >
      <slot :panel="panel" :index="index" />
      <button v-if="editable && !mobile" class="resize-handle no-print" aria-label="调整面板大小" @pointerdown.stop="startInteraction($event, panel, 'resize')" />
    </div>
  </div>
</template>

<style scoped>
.grid-shell {
  touch-action: none;
}

.resize-handle {
  position: absolute;
  right: 5px;
  bottom: 5px;
  width: 16px;
  height: 16px;
  border-right: 2px solid #777ff4;
  border-bottom: 2px solid #777ff4;
  border-radius: 0 0 4px 0;
  background: transparent;
  cursor: nwse-resize;
}
</style>
