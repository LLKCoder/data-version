<script setup lang="ts">
import { Moon, Sun } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'

const { theme, toggleTheme } = useTheme()
</script>

<template>
  <button
    type="button"
    class="theme-toggle"
    :aria-label="theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
    :title="theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
    @click="toggleTheme"
  >
    <Sun v-if="theme === 'dark'" :size="16" />
    <Moon v-else :size="16" />
    <span>{{ theme === 'dark' ? '浅色' : '深色' }}</span>
  </button>
</template>
