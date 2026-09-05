<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ArrowLeft, CheckCircle2, Database, Eye, Pencil, Play, Plus, RefreshCw, Save, Trash2, Wifi, X } from 'lucide-vue-next'
import { datasourceApi } from '@/api/http'
import { queryApi } from '@/api/http'
import type { DataSource, DataSourceInput, QueryResult, TableColumn, TableInfo, TablePreview } from '@/types/dashboard'
import ThemeToggle from '@/components/layout/ThemeToggle.vue'
import SqlEditor, { type SqlDialect } from '@/components/editor/SqlEditor.vue'

type FormState = DataSourceInput & { credentials: { password: string; token: string; headers: Record<string, string> } }

const emptyForm = (): FormState => ({
  name: '',
  type: 'mysql',
  config: { host: '127.0.0.1', port: 3306, database: '', username: '' },
  credentials: { password: '', token: '', headers: {} },
})

const sources = ref<DataSource[]>([])
const selectedUid = ref('')
const editingUid = ref('')
const drawerOpen = ref(false)
const form = reactive<FormState>(emptyForm())
const headersText = ref('{}')
const saving = ref(false)
const testing = ref(false)
const message = ref('')
const error = ref('')
const tableSearch = ref('')
const tables = ref<TableInfo[]>([])
const selectedTable = ref<TableInfo | null>(null)
const columns = ref<TableColumn[]>([])
const preview = ref<TablePreview | null>(null)
const sqlText = ref('')
const sqlResult = ref<QueryResult | null>(null)
const sqlQueryLoading = ref(false)
const sqlQueryError = ref('')
const page = ref(1)
const pageSize = 50
const sortField = ref('')
const sortOrder = ref<'asc' | 'desc'>('asc')

const selectedSource = computed(() => sources.value.find((source) => source.uid === selectedUid.value) ?? null)
const totalPages = computed(() => Math.max(1, Math.ceil((preview.value?.total ?? 0) / pageSize)))
const drawerTitle = computed(() => editingUid.value ? '编辑数据源' : '新增数据源')
const sqlDialect = computed<SqlDialect>(() => selectedSource.value?.type === 'postgres' ? 'postgres' : selectedSource.value?.type === 'sqlite' ? 'sqlite' : 'mysql')

function resetForm() {
  Object.assign(form, emptyForm())
  headersText.value = '{}'
  editingUid.value = ''
}

function fillForm(source: DataSource) {
  editingUid.value = source.uid
  selectedUid.value = source.uid
  form.name = source.name
  form.type = source.type
  form.config = { ...source.config }
  // The API intentionally never returns secrets. Empty values keep existing credentials unchanged on update.
  form.credentials = { password: '', token: '', headers: {} }
  headersText.value = '{}'
}

function openEditor(source?: DataSource) {
  if (source) fillForm(source)
  else {
    resetForm()
    selectedUid.value = ''
  }
  drawerOpen.value = true
  message.value = ''
  error.value = ''
}

function create() {
  openEditor()
}

function closeDrawer() {
  drawerOpen.value = false
  message.value = ''
  error.value = ''
}

function changeType() {
  form.config = form.type === 'http'
    ? { baseUrl: 'http://127.0.0.1:3000' }
    : form.type === 'sqlite'
      ? { path: 'data-vision.sqlite' }
      : { host: '127.0.0.1', port: form.type === 'postgres' ? 5432 : 3306, database: '', username: '', ...(form.type === 'postgres' ? { sslMode: 'disable' } : {}) }
  form.credentials = { password: '', token: '', headers: {} }
  headersText.value = '{}'
}

function payload(): DataSourceInput {
  let headers: Record<string, string> = {}
  try {
    headers = JSON.parse(headersText.value) as Record<string, string>
  } catch {
    // The server remains the source of truth for malformed credential JSON.
  }
  form.credentials.headers = headers
  const value: DataSourceInput = { name: form.name, type: form.type, config: form.config }
  if (form.credentials.password || form.credentials.token || Object.keys(form.credentials.headers).length) value.credentials = form.credentials
  return value
}

async function loadSources() {
  sources.value = await datasourceApi.list()
}

async function save() {
  saving.value = true
  message.value = ''
  error.value = ''
  const wasSelected = selectedUid.value
  try {
    const saved = editingUid.value ? await datasourceApi.update(editingUid.value, payload()) : await datasourceApi.create(payload())
    await loadSources()
    clearTableBrowser()
    selectedUid.value = saved.uid
    drawerOpen.value = false
    message.value = '数据源已保存'
    // Updating the currently selected source does not trigger the selectedUid watcher.
    if (wasSelected === saved.uid && saved.type !== 'http') await loadTables()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  message.value = ''
  error.value = ''
  try {
    const result = await datasourceApi.test(editingUid.value || 'new', payload())
    message.value = result.message
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '连接测试失败'
  } finally {
    testing.value = false
  }
}

async function runSqlQuery() {
  if (!selectedUid.value || selectedSource.value?.type === 'http') return
  sqlQueryLoading.value = true
  sqlQueryError.value = ''
  sqlResult.value = null
  try {
    sqlResult.value = await queryApi.execute({ mode: 'sql', datasourceUid: selectedUid.value, sql: sqlText.value })
  } catch (cause) {
    sqlQueryError.value = cause instanceof Error ? cause.message : 'SQL 查询失败'
  } finally {
    sqlQueryLoading.value = false
  }
}

async function remove(source: DataSource) {
  if (!window.confirm(`删除数据源“${source.name}”？`)) return
  try {
    await datasourceApi.remove(source.uid)
    if (selectedUid.value === source.uid) {
      selectedUid.value = ''
      clearTableBrowser()
    }
    if (editingUid.value === source.uid) closeDrawer()
    await loadSources()
    message.value = '数据源已删除'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '删除失败'
  }
}

function removeSelected() {
  const source = sources.value.find((item) => item.uid === editingUid.value)
  if (source) remove(source)
}

function clearTableBrowser() {
  tables.value = []
  selectedTable.value = null
  columns.value = []
  preview.value = null
  page.value = 1
  sortField.value = ''
  sortOrder.value = 'asc'
  sqlResult.value = null
  sqlQueryError.value = ''
}

async function loadTables() {
  if (!selectedUid.value || selectedSource.value?.type === 'http') return
  try {
    const result = await datasourceApi.tables(selectedUid.value, tableSearch.value)
    tables.value = result.tables
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取数据表失败'
  }
}

async function chooseTable(table: TableInfo) {
  selectedTable.value = table
  page.value = 1
  sortField.value = ''
  sortOrder.value = 'asc'
  if (!sqlText.value.trim()) sqlText.value = `SELECT * FROM ${table.schema}.${table.name}`
  try {
    const schema = await datasourceApi.schema(selectedUid.value, table.schema, table.name)
    columns.value = schema.columns
    await loadPreview()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取表结构失败'
  }
}

async function sortBy(field: string) {
  if (sortField.value === field) sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  else {
    sortField.value = field
    sortOrder.value = 'asc'
  }
  page.value = 1
  await loadPreview()
}

async function loadPreview() {
  if (!selectedTable.value) return
  try {
    preview.value = await datasourceApi.preview(selectedUid.value, { schema: selectedTable.value.schema, table: selectedTable.value.name, page: page.value, pageSize, sort: sortField.value, order: sortOrder.value })
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取表数据失败'
  }
}

function selectSource(source: DataSource) {
  selectedUid.value = source.uid
}

function editSource(source: DataSource) {
  openEditor(source)
}

watch(selectedUid, () => {
  clearTableBrowser()
  sqlText.value = ''
  if (selectedUid.value) loadTables()
})

onMounted(async () => {
  try {
    await loadSources()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取数据源失败'
  }
})
</script>

<template>
  <div class="min-h-screen bg-[#080c19] px-6 py-8 text-[#dce4f7]">
    <div class="mx-auto max-w-[1500px]">
      <header class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <a href="/dashboards/ops-overview" class="mb-3 inline-flex items-center gap-2 text-xs text-[#8893b3] hover:text-white"><ArrowLeft :size="14" />返回看板</a>
          <h1 class="text-3xl font-semibold text-[#f4f5ff]">数据源</h1>
          <p class="mt-2 text-sm text-[#8893b3]">集中管理数据库连接和 HTTP API，连接凭据由服务端加密保存。</p>
        </div>
        <div class="flex items-center gap-2">
          <ThemeToggle />
          <button class="flex items-center gap-2 rounded-xl bg-[#777ff4] px-4 py-2.5 text-xs font-semibold text-white" @click="create">
            <Plus :size="15" />新增数据源
          </button>
        </div>
      </header>

      <div v-if="message" class="mt-5 flex items-center gap-2 rounded-xl border border-[#23554d] bg-[#102c2c] px-4 py-3 text-xs text-[#56e0bd]"><CheckCircle2 :size="15" />{{ message }}</div>
      <div v-if="error" class="mt-5 rounded-xl border border-[#55433b] bg-[#2a201e] px-4 py-3 text-xs text-[#f7b955]">{{ error }}</div>

      <div class="mt-8 grid gap-5 lg:grid-cols-[290px_minmax(0,1fr)]">
        <section class="rounded-2xl border border-[#202945] bg-[#11182b] p-4">
          <div class="mb-3 flex items-center justify-between text-xs uppercase tracking-[0.14em] text-[#687493]">
            <span>已配置 {{ sources.length }}</span>
            <button class="rounded-md p-1 hover:bg-[#151e37]" aria-label="刷新数据源" @click="loadSources"><RefreshCw :size="14" /></button>
          </div>
          <div v-if="!sources.length" class="rounded-xl border border-dashed border-[#2b3554] p-5 text-center text-xs leading-5 text-[#687493]">还没有数据源。<br />点击右上角开始配置。</div>
          <div v-for="source in sources" v-else :key="source.uid" class="mb-2 rounded-xl border px-3 py-3" :class="source.uid === selectedUid ? 'border-[#777ff4] bg-[#1b2340]' : 'border-[#202945]'">
            <button class="w-full text-left" @click="selectSource(source)">
              <div class="flex items-center gap-2 text-sm text-[#eef2ff]"><Database :size="15" class="text-[#8f95ff]" />{{ source.name }}</div>
              <div class="mt-2 flex items-center gap-2 text-[11px] text-[#687493]"><span class="rounded bg-[#202945] px-1.5 py-0.5">{{ source.type }}</span><span :class="source.status === 'connected' ? 'text-[#56e0bd]' : 'text-[#f7b955]'">{{ source.status }}</span></div>
            </button>
            <div class="mt-2 flex justify-end">
              <button class="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-[11px] text-[#8893b3] hover:bg-[#151e37] hover:text-white" :aria-label="`编辑数据源${source.name}`" @click.stop="editSource(source)">
                <Pencil :size="13" />编辑
              </button>
            </div>
          </div>
        </section>

        <section class="rounded-2xl border border-[#202945] bg-[#11182b] p-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-xs uppercase tracking-[0.14em] text-[#687493]">表浏览器</div>
              <h2 class="mt-1 text-lg font-semibold text-[#f4f5ff]">{{ selectedSource?.name ?? '选择数据源' }}</h2>
            </div>
            <div v-if="selectedUid && selectedSource?.type !== 'http'" class="flex items-center gap-2">
              <button class="inline-flex items-center gap-1 rounded-lg border border-[#27345c] px-2.5 py-2 text-xs text-[#8893b3] hover:bg-[#151e37] hover:text-white" @click="editSource(selectedSource!)"><Pencil :size="13" />编辑</button>
              <input v-model="tableSearch" class="field w-44" placeholder="搜索表" @keyup.enter="loadTables" />
              <button class="rounded-lg border border-[#27345c] p-2 text-[#8893b3]" aria-label="搜索数据表" @click="loadTables"><Eye :size="14" /></button>
            </div>
            <button v-else-if="selectedUid" class="inline-flex items-center gap-1 rounded-lg border border-[#27345c] px-2.5 py-2 text-xs text-[#8893b3] hover:bg-[#151e37] hover:text-white" @click="editSource(selectedSource!)"><Pencil :size="13" />编辑</button>
          </div>

          <div v-if="selectedUid && selectedSource?.type !== 'http'" class="mt-6 rounded-xl border border-[#202945] bg-[#151e37] p-4">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div>
                <div class="text-xs uppercase tracking-[0.14em] text-[#687493]">SQL 查询</div>
                <p class="mt-1 text-xs text-[#8893b3]">直接在当前数据源上执行只读查询</p>
              </div>
              <span class="rounded-md bg-[#202945] px-2 py-1 text-[10px] uppercase text-[#aeb7dc]">{{ selectedSource?.type }}</span>
            </div>
            <SqlEditor v-model="sqlText" class="mt-3" :dialect="sqlDialect" :min-height="150" placeholder="SELECT ... WHERE ..." />
            <div class="mt-3 flex items-center justify-between gap-3">
              <span class="text-[11px] text-[#687493]">仅支持 SELECT / WITH；Tab 补全内置语法</span>
              <button class="inline-flex items-center gap-2 rounded-lg bg-[#777ff4] px-3 py-2 text-xs font-semibold text-white hover:brightness-110" :disabled="sqlQueryLoading" @click="runSqlQuery"><Play :size="13" />{{ sqlQueryLoading ? '查询中…' : '执行查询' }}</button>
            </div>
            <div v-if="sqlQueryError" class="mt-3 rounded-lg border border-[#55433b] bg-[#2a201e] px-3 py-2 text-xs leading-5 text-[#f7b955]">{{ sqlQueryError }}</div>
            <div v-if="sqlResult" class="mt-4">
              <div class="mb-2 flex items-center justify-between text-[11px] text-[#687493]"><span>{{ sqlResult.meta.rowCount }} 行 · {{ sqlResult.meta.durationMs }}ms<span v-if="sqlResult.meta.truncated"> · 已截断</span></span><span>查询结果</span></div>
              <div class="max-h-[360px] overflow-auto rounded-lg border border-[#202945]"><table class="w-full min-w-[560px] text-left text-xs"><thead class="sticky top-0 bg-[#151e37] text-[#8893b3]"><tr><th v-for="column in sqlResult.columns" :key="column.name" class="px-3 py-2 font-medium">{{ column.name }} <span class="text-[10px] text-[#687493]">{{ column.type }}</span></th></tr></thead><tbody><tr v-for="(row, index) in sqlResult.rows" :key="index" class="border-t border-[#202945] text-[#c5ccef]"><td v-for="column in sqlResult.columns" :key="column.name" class="max-w-[220px] truncate px-3 py-2">{{ row[column.name] ?? 'NULL' }}</td></tr></tbody></table></div>
            </div>
          </div>

          <div v-if="selectedSource?.type === 'http'" class="mt-10 text-center text-xs text-[#687493]">HTTP 数据源没有数据库表，可在面板编辑器中配置请求。</div>
          <div v-else-if="!selectedUid" class="mt-10 text-center text-xs text-[#687493]">选择左侧数据源查看表和分页数据。</div>
          <template v-else>
            <div class="mt-5 flex max-h-36 flex-wrap content-start gap-2 overflow-auto">
              <button v-for="table in tables" :key="`${table.schema}.${table.name}`" class="rounded-lg border px-3 py-2 text-xs" :class="selectedTable?.name === table.name && selectedTable?.schema === table.schema ? 'border-[#777ff4] bg-[#1b2340] text-white' : 'border-[#202945] text-[#aeb7dc] hover:bg-[#151e37]'" @click="chooseTable(table)">{{ table.schema }}.{{ table.name }}</button>
            </div>
            <div v-if="selectedTable" class="mt-5">
              <div class="flex items-center justify-between">
                <div><h3 class="text-sm font-semibold text-[#eef2ff]">{{ selectedTable.schema }}.{{ selectedTable.name }}</h3><p class="mt-1 text-[11px] text-[#687493]">{{ preview?.total ?? 0 }} 行 · 每页 {{ pageSize }} 行</p></div>
                <div class="flex items-center gap-2"><button class="rounded-lg border border-[#27345c] px-2 py-1 text-xs" :disabled="page <= 1" @click="page--; loadPreview()">上一页</button><span class="text-xs text-[#8893b3]">{{ page }} / {{ totalPages }}</span><button class="rounded-lg border border-[#27345c] px-2 py-1 text-xs" :disabled="page >= totalPages" @click="page++; loadPreview()">下一页</button></div>
              </div>
              <div class="mt-3 overflow-auto rounded-xl border border-[#202945]"><table class="w-full min-w-[500px] text-left text-xs"><thead class="bg-[#151e37] text-[#8893b3]"><tr><th v-for="column in columns" :key="column.name" class="cursor-pointer px-3 py-2" @click="sortBy(column.name)">{{ column.name }} <span class="text-[10px] text-[#687493]">{{ column.type }}<template v-if="sortField === column.name"> · {{ sortOrder === 'asc' ? '↑' : '↓' }}</template></span></th></tr></thead><tbody><tr v-for="(row, index) in preview?.rows ?? []" :key="index" class="border-t border-[#202945] text-[#c5ccef]"><td v-for="column in columns" :key="column.name" class="max-w-[180px] truncate px-3 py-2">{{ row[column.name] ?? 'NULL' }}</td></tr></tbody></table></div>
            </div>
            <div v-else class="mt-10 text-center text-xs text-[#687493]">选择一张表查看字段和数据。</div>
          </template>
        </section>
      </div>
    </div>
  </div>

  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="drawerOpen" class="fixed inset-0 z-40 flex justify-end" @keydown.esc="closeDrawer">
        <button class="drawer-backdrop absolute inset-0 h-full w-full cursor-default" aria-label="关闭数据源编辑抽屉" @click="closeDrawer"></button>
        <aside class="relative z-10 flex h-full w-full max-w-[480px] min-h-0 flex-col border-l border-[#202945] bg-[#0b1020] shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="datasource-drawer-title">
          <div class="flex items-start justify-between border-b border-[#202945] px-6 py-5">
            <div>
              <div class="flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-[#687493]"><Pencil :size="14" />{{ drawerTitle }}</div>
              <h2 id="datasource-drawer-title" class="mt-2 text-xl font-semibold text-[#f4f5ff]">{{ form.name || '新数据源' }}</h2>
              <p class="mt-1 text-xs text-[#687493]">连接信息保存后可随时修改，敏感凭据不会回显。</p>
            </div>
            <button class="rounded-lg p-2 text-[#8893b3] hover:bg-[#151e37] hover:text-white" aria-label="关闭" @click="closeDrawer"><X :size="18" /></button>
          </div>

          <form class="min-h-0 flex-1 overflow-y-auto px-6 py-5" @submit.prevent="save">
            <label class="block text-xs text-[#8893b3]">名称<input v-model="form.name" class="field mt-2" placeholder="业务数据库" autofocus /></label>
            <label class="mt-5 block text-xs text-[#8893b3]">类型<select v-model="form.type" class="field mt-2" @change="changeType"><option value="mysql">MySQL</option><option value="postgres">PostgreSQL</option><option value="sqlite">SQLite</option><option value="http">HTTP API</option></select></label>

            <template v-if="form.type === 'http'">
              <label class="mt-5 block text-xs text-[#8893b3]">Base URL<input v-model="form.config.baseUrl" class="field mt-2" placeholder="https://api.example.com" /></label>
              <label class="mt-5 block text-xs text-[#8893b3]">Token（可选）<input v-model="form.credentials.token" class="field mt-2" type="password" placeholder="更新时填写，读取时不会回显" /></label>
              <label class="mt-5 block text-xs text-[#8893b3]">凭据请求头 JSON<textarea v-model="headersText" class="field mt-2 font-mono text-xs" rows="4" placeholder="{ &quot;X-API-Key&quot;: &quot;...&quot; }" /></label>
            </template>
            <template v-else-if="form.type === 'sqlite'">
              <label class="mt-5 block text-xs text-[#8893b3]">文件路径<input v-model="form.config.path" class="field mt-2" placeholder="/data/app.sqlite" /></label>
            </template>
            <template v-else>
              <div class="mt-5 grid grid-cols-3 gap-3"><label class="col-span-2 text-xs text-[#8893b3]">主机<input v-model="form.config.host" class="field mt-2" /></label><label class="text-xs text-[#8893b3]">端口<input v-model.number="form.config.port" class="field mt-2" type="number" /></label></div>
              <label class="mt-5 block text-xs text-[#8893b3]">数据库<input v-model="form.config.database" class="field mt-2" /></label>
              <label class="mt-5 block text-xs text-[#8893b3]">用户名<input v-model="form.config.username" class="field mt-2" /></label>
              <label v-if="form.type === 'postgres'" class="mt-5 block text-xs text-[#8893b3]">SSL 模式<input v-model="form.config.sslMode" class="field mt-2" /></label>
              <label class="mt-5 block text-xs text-[#8893b3]">密码（{{ editingUid ? '留空表示不修改' : '可选' }}）<input v-model="form.credentials.password" class="field mt-2" type="password" /></label>
            </template>
          </form>

          <div class="border-t border-[#202945] px-6 py-4">
            <div v-if="error" class="mb-3 rounded-lg border border-[#55433b] bg-[#2a201e] px-3 py-2 text-xs leading-5 text-[#f7b955]">{{ error }}</div>
            <div v-if="message" class="mb-3 flex items-center gap-2 rounded-lg border border-[#23554d] bg-[#102c2c] px-3 py-2 text-xs text-[#56e0bd]"><CheckCircle2 :size="14" />{{ message }}</div>
            <div class="flex gap-2">
              <button type="button" class="flex flex-1 items-center justify-center gap-2 rounded-xl border border-[#27345c] px-3 py-2.5 text-xs text-[#c5ccef] hover:bg-[#151e37]" :disabled="testing || saving" @click="test"><Wifi :size="14" />{{ testing ? '测试中…' : '测试连接' }}</button>
              <button type="submit" class="flex flex-1 items-center justify-center gap-2 rounded-xl bg-[#777ff4] px-3 py-2.5 text-xs font-semibold text-white hover:brightness-110" :disabled="saving || testing"><Save :size="14" />{{ saving ? '保存中…' : '保存配置' }}</button>
            </div>
            <button v-if="editingUid" type="button" class="mt-3 flex w-full items-center justify-center gap-2 rounded-xl px-3 py-2 text-xs text-[#f4779f] hover:bg-[#321d35]" :disabled="saving || testing" @click="removeSelected"><Trash2 :size="14" />删除数据源</button>
          </div>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.field { width: 100%; border: 1px solid var(--app-border-strong); border-radius: 0.65rem; background: var(--app-surface); color: var(--app-text); padding: 0.55rem 0.75rem; line-height: 1.35; outline: none; transition: border-color 160ms ease, box-shadow 160ms ease; }
.field:focus { border-color: var(--app-primary); box-shadow: 0 0 0 3px color-mix(in srgb, var(--app-primary) 16%, transparent); }
.drawer-backdrop { background: rgba(4, 7, 16, 0.58); }
.drawer-enter-active, .drawer-leave-active { transition: opacity 180ms ease; }
.drawer-enter-active aside, .drawer-leave-active aside { transition: transform 220ms ease; }
.drawer-enter-from, .drawer-leave-to { opacity: 0; }
.drawer-enter-from aside, .drawer-leave-to aside { transform: translateX(100%); }
</style>
