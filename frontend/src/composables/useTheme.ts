import { computed, ref } from 'vue'

export type Theme = 'dark' | 'light'

const STORAGE_KEY = 'data-vision-theme'

function readTheme(): Theme {
  if (typeof window === 'undefined') return 'dark'

  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') return stored
  } catch {
    // Ignore storage restrictions and keep the dark theme as the fallback.
  }

  return 'dark'
}

const theme = ref<Theme>(readTheme())

function applyTheme(nextTheme: Theme) {
  if (typeof document === 'undefined') return

  const root = document.documentElement
  root.dataset.theme = nextTheme
  root.style.colorScheme = nextTheme

  const themeColor = document.querySelector('meta[name="theme-color"]')
  themeColor?.setAttribute('content', nextTheme === 'dark' ? '#0b1020' : '#f5f7fb')
}

function setTheme(nextTheme: Theme) {
  theme.value = nextTheme
  applyTheme(nextTheme)

  try {
    window.localStorage.setItem(STORAGE_KEY, nextTheme)
  } catch {
    // The theme still works for the current session when storage is unavailable.
  }
}

function toggleTheme() {
  setTheme(theme.value === 'dark' ? 'light' : 'dark')
}

applyTheme(theme.value)

export function useTheme() {
  return {
    theme,
    isDark: computed(() => theme.value === 'dark'),
    setTheme,
    toggleTheme,
  }
}
