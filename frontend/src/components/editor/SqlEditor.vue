<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'

export type SqlDialect = 'mysql' | 'postgres' | 'sqlite'

const props = withDefaults(defineProps<{
  modelValue: string
  dialect?: SqlDialect
  placeholder?: string
  minHeight?: number
}>(), {
  dialect: 'mysql',
  placeholder: 'SELECT ...',
  minHeight: 180,
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const input = ref<HTMLTextAreaElement | null>(null)
const highlightLayer = ref<HTMLElement | null>(null)
const cursorPosition = ref(0)
const focused = ref(false)
const suggestionIndex = ref(0)

const commonKeywords = [
  'SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'NOT', 'NULL', 'IS', 'IN', 'LIKE', 'BETWEEN',
  'AS', 'ON', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'OUTER', 'CROSS', 'FULL', 'GROUP', 'BY',
  'ORDER', 'ASC', 'DESC', 'LIMIT', 'OFFSET', 'HAVING', 'DISTINCT', 'UNION', 'ALL', 'WITH',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'OVER', 'PARTITION', 'ROWS', 'RANGE', 'CURRENT',
  'ROW', 'FETCH', 'FIRST', 'ONLY', 'EXISTS', 'TRUE', 'FALSE',
]

const commonFunctions = [
  'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'COALESCE', 'NULLIF', 'CAST', 'CONVERT', 'ROUND',
  'ABS', 'CEIL', 'CEILING', 'FLOOR', 'POWER', 'MOD', 'LOWER', 'UPPER', 'LENGTH', 'TRIM',
  'SUBSTR', 'SUBSTRING', 'REPLACE', 'CONCAT', 'COUNT', 'CURRENT_DATE', 'CURRENT_TIME',
  'CURRENT_TIMESTAMP', 'DATE', 'TIME', 'YEAR', 'MONTH', 'DAY',
]

const dialectCompletions: Record<SqlDialect, string[]> = {
  mysql: ['IF', 'IFNULL', 'GROUP_CONCAT', 'JSON_EXTRACT', 'DATE_FORMAT', 'REGEXP'],
  postgres: ['ILIKE', 'STRING_AGG', 'DATE_TRUNC', 'EXTRACT', 'REGEXP_MATCHES', 'RETURNING'],
  sqlite: ['IFNULL', 'GROUP_CONCAT', 'GLOB', 'STRFTIME', 'JULIANDAY', 'TOTAL'],
}

const completionWords = computed(() => [...new Set([
  ...commonKeywords,
  ...commonFunctions,
  ...dialectCompletions[props.dialect],
])])

const currentPrefix = computed(() => {
  const beforeCursor = props.modelValue.slice(0, cursorPosition.value)
  return beforeCursor.match(/[A-Za-z_][A-Za-z0-9_]*$/)?.[0].toUpperCase() ?? ''
})

const suggestions = computed(() => {
  if (!focused.value || !currentPrefix.value) return []
  return completionWords.value.filter((word) => word.startsWith(currentPrefix.value)).slice(0, 6)
})

const highlightedSql = computed(() => highlightSql(props.modelValue))
const editorStyle = computed(() => ({ minHeight: `${props.minHeight}px` }))

function escapeHtml(value: string) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;')
}

function highlightSql(value: string) {
  const keywordSet = new Set(commonKeywords)
  const functionSet = new Set([...commonFunctions, ...dialectCompletions[props.dialect]])
  let html = ''
  let index = 0

  while (index < value.length) {
    const rest = value.slice(index)
    const comment = rest.match(/^--[^\n]*/)
    if (comment) {
      html += `<span class="token-comment">${escapeHtml(comment[0])}</span>`
      index += comment[0].length
      continue
    }
    const blockComment = rest.match(/^\/\*[\s\S]*?\*\//)
    if (blockComment) {
      html += `<span class="token-comment">${escapeHtml(blockComment[0])}</span>`
      index += blockComment[0].length
      continue
    }
    const quote = value[index]
    if (quote === '\'' || quote === '"' || quote === '`') {
      let end = index + 1
      while (end < value.length) {
        if (value[end] === quote) {
          if (value[end + 1] === quote) {
            end += 2
            continue
          }
          end++
          break
        }
        if (value[end] === '\\' && end + 1 < value.length) end += 2
        else end++
      }
      html += `<span class="token-string">${escapeHtml(value.slice(index, end))}</span>`
      index = end
      continue
    }
    const number = rest.match(/^(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?/i)
    if (number) {
      html += `<span class="token-number">${escapeHtml(number[0])}</span>`
      index += number[0].length
      continue
    }
    const word = rest.match(/^[A-Za-z_][A-Za-z0-9_$]*/)
    if (word) {
      const upper = word[0].toUpperCase()
      const afterWord = value.slice(index + word[0].length).match(/^\s*\(/)
      const tokenClass = functionSet.has(upper) && afterWord ? 'token-function' : keywordSet.has(upper) ? 'token-keyword' : ''
      html += tokenClass ? `<span class="${tokenClass}">${escapeHtml(word[0])}</span>` : escapeHtml(word[0])
      index += word[0].length
      continue
    }
    const operator = rest.match(/^(?:<>|!=|<=|>=|:=|\|\||[-+*/%=<>])/)
    if (operator) {
      html += `<span class="token-operator">${escapeHtml(operator[0])}</span>`
      index += operator[0].length
      continue
    }
    html += escapeHtml(value[index])
    index++
  }

  return html || ' '
}

function syncCursor() {
  cursorPosition.value = input.value?.selectionStart ?? props.modelValue.length
  suggestionIndex.value = 0
}

function syncScroll(event: Event) {
  const target = event.target as HTMLTextAreaElement
  if (!highlightLayer.value) return
  highlightLayer.value.scrollTop = target.scrollTop
  highlightLayer.value.scrollLeft = target.scrollLeft
}

function updateValue(event: Event) {
  const target = event.target as HTMLTextAreaElement
  cursorPosition.value = target.selectionStart
  suggestionIndex.value = 0
  emit('update:modelValue', target.value)
}

function replaceRange(start: number, end: number, replacement: string) {
  const nextValue = `${props.modelValue.slice(0, start)}${replacement}${props.modelValue.slice(end)}`
  emit('update:modelValue', nextValue)
  nextTick(() => {
    if (!input.value) return
    const nextCursor = start + replacement.length
    input.value.focus()
    input.value.setSelectionRange(nextCursor, nextCursor)
    cursorPosition.value = nextCursor
  })
}

function acceptSuggestion() {
  const position = input.value?.selectionStart ?? cursorPosition.value
  const match = props.modelValue.slice(0, position).match(/[A-Za-z_][A-Za-z0-9_]*$/)
  if (!match || !suggestions.value.length) return false
  const start = position - match[0].length
  const suggestion = suggestions.value[suggestionIndex.value] ?? suggestions.value[0]
  const nextCharacter = props.modelValue[position] ?? ''
  const suffix = nextCharacter && /[A-Za-z0-9_$]/.test(nextCharacter) ? '' : ' '
  replaceRange(start, position, `${suggestion}${suffix}`)
  return true
}

function insertIndent() {
  const position = input.value?.selectionStart ?? cursorPosition.value
  replaceRange(position, position, '  ')
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Tab') {
    event.preventDefault()
    if (!acceptSuggestion()) insertIndent()
    return
  }
  if (suggestions.value.length && event.key === 'ArrowDown') {
    event.preventDefault()
    suggestionIndex.value = (suggestionIndex.value + 1) % suggestions.value.length
  } else if (suggestions.value.length && event.key === 'ArrowUp') {
    event.preventDefault()
    suggestionIndex.value = (suggestionIndex.value - 1 + suggestions.value.length) % suggestions.value.length
  }
}
</script>

<template>
  <div class="sql-editor" :class="{ 'is-focused': focused }" :style="editorStyle">
    <pre ref="highlightLayer" class="sql-highlight" aria-hidden="true" v-html="highlightedSql"></pre>
    <textarea
      ref="input"
      class="sql-input"
      :value="modelValue"
      :placeholder="placeholder"
      :aria-label="placeholder"
      spellcheck="false"
      wrap="off"
      @input="updateValue"
      @keydown="handleKeydown"
      @keyup="syncCursor"
      @click="syncCursor"
      @select="syncCursor"
      @scroll="syncScroll"
      @focus="focused = true; syncCursor()"
      @blur="focused = false"
    />
    <div v-if="focused && suggestions.length" class="completion-hint">
      <span>Tab 补全</span>
      <strong>{{ suggestions[suggestionIndex] }}</strong>
      <span v-if="suggestions.length > 1">+{{ suggestions.length - 1 }}</span>
    </div>
  </div>
</template>

<style scoped>
.sql-editor { position: relative; overflow: hidden; border: 1px solid var(--app-border-strong); border-radius: 0.65rem; background: var(--app-surface); transition: border-color 160ms ease, box-shadow 160ms ease; }
.sql-editor.is-focused { border-color: var(--app-primary); box-shadow: 0 0 0 3px color-mix(in srgb, var(--app-primary) 16%, transparent); }
.sql-highlight, .sql-input { margin: 0; padding: 0.65rem 0.85rem; font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace; font-size: 0.78rem; line-height: 1.55; tab-size: 2; }
.sql-highlight { position: absolute; inset: 0; overflow: hidden; color: var(--app-text-input); pointer-events: none; white-space: pre; }
.sql-input { position: relative; display: block; width: 100%; min-height: inherit; resize: vertical; border: 0; outline: 0; background: transparent; color: transparent; caret-color: var(--app-text); -webkit-text-fill-color: transparent; }
.sql-input::selection { background: color-mix(in srgb, var(--app-primary) 38%, transparent); }
.sql-input::placeholder { color: var(--app-text-subtle); -webkit-text-fill-color: var(--app-text-subtle); }
.completion-hint { position: absolute; right: 0.65rem; bottom: 0.55rem; display: flex; align-items: center; gap: 0.4rem; border: 1px solid var(--app-border-strong); border-radius: 0.45rem; background: var(--app-surface-subtle); padding: 0.28rem 0.5rem; color: var(--app-text-subtle); font: 0.68rem/1.2 ui-sans-serif, system-ui, sans-serif; pointer-events: none; }
.completion-hint strong { color: var(--app-primary-muted); font-weight: 600; }
:deep(.token-keyword) { color: #8f95ff; font-weight: 600; }
:deep(.token-function) { color: #56e0bd; }
:deep(.token-string) { color: #f7b955; }
:deep(.token-number) { color: #57b6f7; }
:deep(.token-comment) { color: var(--app-text-subtle); font-style: italic; }
:deep(.token-operator) { color: #f4779f; }
</style>
