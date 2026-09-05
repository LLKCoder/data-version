import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { dashboardApi } from '@/api/http'
import type { Dashboard } from '@/types/dashboard'

export const useDashboardStore = defineStore('dashboard', () => {
  const current = ref<Dashboard | null>(null)
  const loading = ref(false)
  const error = ref('')
  const lastUpdated = ref(new Date())

  const panels = computed(() => current.value?.panels ?? [])

  async function load(uid: string) {
    loading.value = true
    error.value = ''
    current.value = null
    try {
      current.value = await dashboardApi.get(uid)
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '看板加载失败'
    } finally {
      lastUpdated.value = new Date()
      loading.value = false
    }
  }

  async function save(dashboard: Dashboard) {
    loading.value = true
    error.value = ''
    try {
      current.value = await dashboardApi.update(dashboard.uid, dashboard)
      lastUpdated.value = new Date()
      return current.value
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : '看板保存失败'
      throw cause
    } finally {
      loading.value = false
    }
  }

  return { current, panels, loading, error, lastUpdated, load, save }
})
