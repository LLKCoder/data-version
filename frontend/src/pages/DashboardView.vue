<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Bell, ChevronDown, Clock3, Download, LayoutDashboard, PanelLeftClose, PanelLeftOpen, Plus, RefreshCw, Settings2, Share2, Star, Upload } from 'lucide-vue-next'
import { dashboardApi, datasourceApi, queryApi } from '@/api/http'
import type { Dashboard, DataSource, Panel, QueryConfig, QueryResult } from '@/types/dashboard'
import ChartPanel from '@/components/panel/ChartPanel.vue'
import GridLayout from '@/components/layout/GridLayout.vue'
import ThemeToggle from '@/components/layout/ThemeToggle.vue'
import SqlEditor, { type SqlDialect } from '@/components/editor/SqlEditor.vue'
import { useDashboardStore } from '@/stores/dashboard'

const route = useRoute()
const router = useRouter()
const store = useDashboardStore()
const sidebarOpen = ref(true)
const editing = ref(route.query.edit === '1')
const draft = ref<Dashboard | null>(null)
const baseline = ref('')
const dataSources = ref<DataSource[]>([])
const dashboards = ref<Dashboard[]>([])
const selectedPanelUid = ref('')
const pipelineText = ref<Record<string, string>>({})
const results = ref<Record<string, QueryResult | null>>({})
const queryLoading = ref<Record<string, boolean>>({})
const queryErrors = ref<Record<string, string>>({})
const exporting = ref(false)
const importing = ref(false)
const saveError = ref('')
let refreshTimer: number | undefined

const uid = computed(() => String(route.params.uid ?? 'ops-overview'))
const isNew = computed(() => uid.value === 'new')
const dashboard = computed(() => draft.value ?? store.current)
const selectedPanel = computed(() => draft.value?.panels.find((panel) => panel.uid === selectedPanelUid.value) ?? null)
const selectedQuery = computed(() => selectedPanel.value?.queryConfig ?? { mode: 'none' } as QueryConfig)
const selectedRequest = computed(() => selectedQuery.value.request ?? { method: 'GET', path: '/', rowsPath: '', fieldMap: {} })
const selectedResult = computed(() => selectedPanel.value ? results.value[selectedPanel.value.uid] ?? null : null)
const selectedSqlDialect = computed<SqlDialect>(() => {
  const source = dataSources.value.find((item) => item.uid === selectedQuery.value.datasourceUid)
  return source?.type === 'postgres' ? 'postgres' : source?.type === 'sqlite' ? 'sqlite' : 'mysql'
})
const fieldMapText = computed({
  get: () => JSON.stringify(selectedRequest.value.fieldMap ?? {}, null, 2),
  set: (value: string) => { try { selectedRequest.value.fieldMap = JSON.parse(value) as Record<string, string> } catch { /* keep the text valid before applying it */ } },
})
const requestParamsText = computed({
	get: () => JSON.stringify(selectedRequest.value.params ?? {}, null, 2),
	set: (value: string) => { try { selectedRequest.value.params = JSON.parse(value) as Record<string, unknown> } catch { /* keep the current valid request until JSON is fixed */ } },
})
const requestBodyText = computed({
	get: () => selectedRequest.value.body === undefined ? '' : JSON.stringify(selectedRequest.value.body, null, 2),
	set: (value: string) => { try { selectedRequest.value.body = value.trim() === '' ? undefined : JSON.parse(value) } catch { /* keep the current valid request until JSON is fixed */ } },
})
const sqlParamsText = computed({
	get: () => JSON.stringify(selectedQuery.value.params ?? {}, null, 2),
	set: (value: string) => { try { selectedQuery.value.params = JSON.parse(value) as Record<string, unknown> } catch { /* keep the current valid query until JSON is fixed */ } },
})
const dirty = computed(() => Boolean(draft.value && baseline.value && JSON.stringify(draft.value) !== baseline.value))
const lastUpdatedLabel = computed(() => store.lastUpdated.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }))

function cloneDashboard(value: Dashboard): Dashboard { return JSON.parse(JSON.stringify(value)) as Dashboard }
function defaultPanel(): Panel { return { uid: `panel-${Date.now()}`, title: '新面板', type: 'timeseries', x: 0, y: 0, w: 6, h: 4, queryConfig: { mode: 'none' }, visualization: {} } }

async function loadDashboard() {
  if (isNew.value) {
    draft.value = { uid: 'new', name: '新看板', description: '', timeRange: '最近 24 小时', refreshInterval: 30, revision: 1, panels: [] }
    baseline.value = JSON.stringify(draft.value)
    editing.value = true
    return
  }
  draft.value = null
  await store.load(uid.value)
  if (store.current) {
    draft.value = cloneDashboard(store.current)
    baseline.value = JSON.stringify(draft.value)
    await runQueries()
  }
}

async function runQueries() {
  const panels = dashboard.value?.panels ?? []
  await Promise.all(panels.map(async (panel) => {
    const config = panel.queryConfig
    if (!config || config.mode === 'none' || !config.mode) { results.value[panel.uid] = null; queryErrors.value[panel.uid] = ''; return }
    queryLoading.value[panel.uid] = true
    queryErrors.value[panel.uid] = ''
    try { results.value[panel.uid] = await queryApi.execute(config) } catch (cause) { queryErrors.value[panel.uid] = cause instanceof Error ? cause.message : '查询失败' } finally { queryLoading.value[panel.uid] = false }
  }))
}

async function runPanel(panel: Panel) {
  const config = panel.queryConfig
  if (!config || config.mode === 'none') { queryErrors.value[panel.uid] = '请先配置查询'; return }
  queryLoading.value[panel.uid] = true
  queryErrors.value[panel.uid] = ''
  try { results.value[panel.uid] = await queryApi.execute(config) } catch (cause) { queryErrors.value[panel.uid] = cause instanceof Error ? cause.message : '查询失败' } finally { queryLoading.value[panel.uid] = false }
}

function enterEdit() { if (store.current) { draft.value = cloneDashboard(store.current); editing.value = true; router.replace({ query: { ...route.query, edit: '1' } }) } }
function cancelEdit() { if (dirty.value && !window.confirm('当前有未保存修改，确定放弃吗？')) return; if (store.current) draft.value = cloneDashboard(store.current); editing.value = false; selectedPanelUid.value = ''; router.replace({ query: { ...route.query, edit: undefined } }) }
async function saveDashboard() {
  if (!draft.value) return
  saveError.value = ''
  try {
    if (isNew.value) {
      const saved = await dashboardApi.create(draft.value)
      draft.value = cloneDashboard(saved)
      baseline.value = JSON.stringify(draft.value)
      await router.replace(`/dashboards/${saved.uid}?edit=1`)
      editing.value = false
      await runQueries()
    } else {
      const saved = await store.save(draft.value)
      draft.value = cloneDashboard(saved)
      baseline.value = JSON.stringify(draft.value)
      editing.value = false
      router.replace({ query: { ...route.query, edit: undefined } })
      await runQueries()
    }
  } catch (cause) { saveError.value = cause instanceof Error ? cause.message : '保存失败' }
}
function addPanel() { if (!draft.value) return; const panel = defaultPanel(); draft.value.panels.push(panel); selectedPanelUid.value = panel.uid }
function removePanel(panel: Panel) { if (!draft.value || !window.confirm(`删除面板“${panel.title}”？`)) return; draft.value.panels = draft.value.panels.filter((item) => item.uid !== panel.uid); selectedPanelUid.value = '' }
function duplicatePanel(panel: Panel) { if (!draft.value) return; const copy = cloneDashboard({ ...draft.value, panels: [panel] }).panels[0]; copy.uid = `panel-${Date.now()}`; copy.title = `${panel.title}（副本）`; copy.x = Math.min((panel.x || 0) + 1, 10); copy.y = (panel.y || 0) + 1; draft.value.panels.push(copy) }
function editPanel(panel: Panel) { selectedPanelUid.value = panel.uid }
function updateLayout(panels: Panel[]) { if (draft.value) draft.value.panels = panels }

function queryMode(panel: Panel) { return panel.queryConfig?.mode ?? 'none' }
function setQueryMode(panel: Panel, mode: QueryConfig['mode']) {
  if (mode === 'sql') panel.queryConfig = { mode, datasourceUid: dataSources.value.find((source) => source.type !== 'http')?.uid ?? '', sql: 'SELECT 1 AS value' }
  else if (mode === 'http') panel.queryConfig = { mode, datasourceUid: dataSources.value.find((source) => source.type === 'http')?.uid ?? '', request: { method: 'GET', path: '/', rowsPath: '' } }
  else if (mode === 'pipeline') panel.queryConfig = { mode, nodes: [], outputNodeId: '' }
  else panel.queryConfig = { mode: 'none' }
}
function setPipelineText(panel: Panel, value: string) { pipelineText.value[panel.uid] = value; try { panel.queryConfig = JSON.parse(value) as QueryConfig } catch { /* keep editor text until the user fixes JSON */ } }
function ensurePipelineText(panel: Panel) { if (!pipelineText.value[panel.uid]) pipelineText.value[panel.uid] = JSON.stringify(panel.queryConfig ?? { mode: 'pipeline', nodes: [], outputNodeId: '' }, null, 2); return pipelineText.value[panel.uid] }
function setVisualization(panel: Panel, key: 'xField' | 'yField' | 'valueField' | 'nameField', value: string) { panel.visualization = { ...(panel.visualization ?? {}), [key]: value } }
function sourceOptions(type?: 'http' | 'db') { return dataSources.value.filter((source) => type === 'http' ? source.type === 'http' : source.type !== 'http') }
function eventValue(event: Event) { return (event.target as HTMLSelectElement).value }
function changeMode(panel: Panel, event: Event) { setQueryMode(panel, eventValue(event) as QueryConfig['mode']) }
function changeType(panel: Panel, event: Event) { panel.type = eventValue(event) }
function changeField(panel: Panel, key: 'xField' | 'yField', event: Event) { setVisualization(panel, key, eventValue(event)) }
function changePipeline(panel: Panel, event: Event) { setPipelineText(panel, (event.target as HTMLTextAreaElement).value) }

async function exportPdf() {
  exporting.value = true
  try { const response = await dashboardApi.exportPdf(uid.value); if (!response.ok) throw new Error('PDF 服务暂不可用'); const blob = await response.blob(); downloadBlob(blob, `${dashboard.value?.name ?? 'dashboard'}.pdf`) } catch { window.print() } finally { exporting.value = false }
}
async function exportJson() { const response = await dashboardApi.exportJson(uid.value); if (!response.ok) throw new Error('JSON 导出失败'); downloadBlob(await response.blob(), `${dashboard.value?.name ?? 'dashboard'}.json`) }
function downloadBlob(blob: Blob, filename: string) { const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = filename; anchor.click(); URL.revokeObjectURL(url) }
async function importJson(event: Event) { const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return; importing.value = true; try { const imported = await dashboardApi.importJson(JSON.parse(await file.text())); await router.push(`/dashboards/${imported.dashboard.uid}?edit=1`); await loadDashboard() } catch (cause) { saveError.value = cause instanceof Error ? cause.message : 'JSON 导入失败' } finally { importing.value = false; (event.target as HTMLInputElement).value = '' } }

watch(() => route.query.edit, (value) => { editing.value = value === '1' })
watch(() => uid.value, () => { loadDashboard() })
onMounted(async () => {
  try { dataSources.value = await datasourceApi.list(); dashboards.value = await dashboardApi.list() } catch { /* dashboard errors are shown by the store */ }
  await loadDashboard()
  refreshTimer = window.setInterval(() => { if (!editing.value) runQueries() }, Math.max((dashboard.value?.refreshInterval ?? 30), 10) * 1000)
})
onUnmounted(() => { if (refreshTimer) window.clearInterval(refreshTimer) })
</script>

<template>
  <div class="min-h-screen bg-transparent text-[#dce4f7]">
    <aside v-if="sidebarOpen" class="no-print fixed inset-y-0 left-0 z-20 flex w-[232px] flex-col border-r border-[#1b2440] bg-[#0b1020]/95 px-4 py-5 backdrop-blur-xl">
      <div class="flex items-center gap-3 px-2"><div class="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-[#878dff] to-[#4b54c7] shadow-lg shadow-indigo-950/50"><LayoutDashboard :size="18" /></div><div><div class="text-sm font-bold tracking-wide text-[#f4f5ff]">DATA VISION</div><div class="text-[10px] tracking-[0.18em] text-[#687493]">OBSERVABILITY</div></div></div>
      <div class="mt-10 px-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-[#687493]">工作区</div>
      <nav class="mt-3 space-y-1"><a class="flex items-center gap-3 rounded-xl bg-[#1b2340] px-3 py-2.5 text-sm font-medium text-[#f0f2ff]" href="#"><LayoutDashboard :size="16" class="text-[#8f95ff]" />所有看板</a><a class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm text-[#8893b3]" href="#"><Star :size="16" />收藏夹</a><a class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm text-[#8893b3]" href="/datasources"><Settings2 :size="16" />数据源</a><a class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm text-[#8893b3]" href="#"><Bell :size="16" />告警</a></nav>
      <div class="mt-9 flex items-center justify-between px-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-[#687493]"><span>所有看板</span><a href="/dashboards/new" class="text-[#8893b3] hover:text-white"><Plus :size="14" /></a></div>
      <div class="mt-3 max-h-64 space-y-1 overflow-auto"><a v-for="item in dashboards" :key="item.uid" class="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm" :class="item.uid === uid ? 'bg-[#151e37] text-[#dce4f7]' : 'text-[#8893b3]'" :href="`/dashboards/${item.uid}`"><span class="h-2 w-2 rounded-full bg-[#3dd9b4]"></span>{{ item.name }}</a></div>
      <div class="mt-auto rounded-2xl border border-[#202945] bg-[#11182b] p-3"><div class="flex items-center gap-2 text-xs font-medium text-[#dce4f7]"><span class="h-2 w-2 rounded-full bg-[#3dd9b4] shadow-[0_0_10px_#3dd9b4]"></span>系统运行正常</div><div class="mt-2 text-[11px] leading-5 text-[#687493]">数据源连接状态可在数据源页面查看。</div></div>
    </aside>

    <main class="print-page transition-[margin] duration-300" :class="sidebarOpen ? 'ml-[232px]' : 'ml-0'">
      <header class="no-print sticky top-0 z-10 flex h-[72px] items-center justify-between border-b border-[#1b2440] bg-[#080c19]/80 px-7 backdrop-blur-xl"><div class="flex items-center gap-4"><button class="rounded-lg p-2 text-[#8893b3] hover:bg-[#141d36]" aria-label="切换侧边栏" @click="sidebarOpen = !sidebarOpen"><PanelLeftClose v-if="sidebarOpen" :size="18" /><PanelLeftOpen v-else :size="18" /></button><div class="h-5 w-px bg-[#202945]"></div><div class="flex items-center gap-2 text-sm text-[#8893b3]"><span>看板</span><ChevronDown :size="14" /><span class="text-[#dce4f7]">{{ dashboard?.name ?? '加载中…' }}</span></div></div><div class="flex items-center gap-2"><ThemeToggle /><a href="/datasources" class="rounded-xl p-2.5 text-[#8893b3] hover:bg-[#141d36]" aria-label="数据源设置"><Settings2 :size="17" /></a><div class="ml-1 flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-[#f7b955] to-[#f4779f] text-xs font-bold text-[#1a1524]">DV</div></div></header>
      <div class="mx-auto max-w-[1600px] px-7 py-8">
        <div class="no-print flex flex-wrap items-center justify-between gap-4"><div><div class="mb-2 flex items-center gap-2 text-xs text-[#687493]"><span>工作区</span><span>/</span><span>所有看板</span></div><h1 class="text-3xl font-semibold tracking-tight text-[#f4f5ff]">{{ dashboard?.name ?? '看板不存在' }}</h1><p class="mt-2 text-sm text-[#8893b3]">{{ dashboard?.description ?? store.error }}</p></div><div class="flex flex-wrap items-center gap-2"><button class="flex items-center gap-2 rounded-xl border border-[#202945] bg-[#11182b] px-3.5 py-2.5 text-xs font-medium text-[#c5ccef]"><Clock3 :size="15" />{{ dashboard?.timeRange ?? '最近 24 小时' }}</button><button class="flex items-center gap-2 rounded-xl border border-[#202945] bg-[#11182b] px-3.5 py-2.5 text-xs font-medium text-[#c5ccef]" @click="runQueries"><RefreshCw :size="15" :class="store.loading ? 'animate-spin' : ''" />刷新</button><button v-if="!editing" class="rounded-xl bg-[#777ff4] px-3.5 py-2.5 text-xs font-semibold text-white" @click="enterEdit">编辑看板</button><button v-else class="rounded-xl bg-[#3dd9b4] px-3.5 py-2.5 text-xs font-semibold text-[#07151a]" :disabled="!dirty || store.loading" @click="saveDashboard">{{ store.loading ? '保存中…' : '保存看板' }}</button><button v-if="editing" class="rounded-xl border border-[#202945] px-3.5 py-2.5 text-xs text-[#c5ccef]" @click="cancelEdit">放弃修改</button><button class="rounded-xl border border-[#202945] bg-[#11182b] p-2.5 text-[#8893b3]" title="导出 JSON" @click="exportJson"><Download :size="17" /></button><label class="rounded-xl border border-[#202945] bg-[#11182b] p-2.5 text-[#8893b3]" title="导入 JSON"><Upload :size="17" /><input class="hidden" type="file" accept="application/json" :disabled="importing" @change="importJson"></label><button class="rounded-xl border border-[#202945] bg-[#11182b] p-2.5 text-[#8893b3]" title="导出 PDF" @click="exportPdf"><Download :size="17" /></button></div></div>
        <div class="no-print mt-8 flex flex-wrap items-center gap-3 border-b border-[#1b2440] pb-4"><div class="flex items-center gap-2 rounded-lg bg-[#151e37] px-3 py-2 text-xs text-[#c5ccef]"><span class="text-[#687493]">数据源</span><span>{{ dataSources.length }} 个已配置</span></div><div class="ml-auto flex items-center gap-2 text-[11px] text-[#687493]"><span class="h-1.5 w-1.5 rounded-full bg-[#3dd9b4]"></span>实时数据 · {{ lastUpdatedLabel }} 更新<span v-if="dirty" class="ml-3 text-[#f7b955]">有未保存修改</span></div></div>
        <div v-if="store.error" class="no-print mt-4 rounded-xl border border-[#55433b] bg-[#2a201e] px-4 py-3 text-xs text-[#f7b955]">{{ store.error }}</div><div v-if="saveError" class="no-print mt-4 rounded-xl border border-[#55433b] bg-[#2a201e] px-4 py-3 text-xs text-[#f7b955]">{{ saveError }}</div>
        <div v-if="editing" class="no-print mt-4 flex items-center justify-between rounded-xl border border-[#27345c] bg-[#11182b] px-4 py-3 text-xs text-[#aeb7dc]"><span>编辑模式：拖动面板标题区域调整位置，拖动右下角调整大小。</span><button class="flex items-center gap-2 rounded-lg bg-[#777ff4] px-3 py-2 font-semibold text-white" @click="addPanel"><Plus :size="14" />新增面板</button></div>

        <section v-if="dashboard" class="mt-5" :class="{ 'opacity-60': store.loading && !draft }"><GridLayout :panels="dashboard.panels" :editable="editing" @update="updateLayout"><template #default="{ panel }"><ChartPanel :panel="panel" :result="results[panel.uid]" :loading="queryLoading[panel.uid]" :error="queryErrors[panel.uid]" :editable="editing" @edit="editPanel(panel)" @remove="removePanel(panel)" @duplicate="duplicatePanel(panel)" /></template></GridLayout></section>
        <footer class="no-print mt-8 flex items-center justify-between border-t border-[#1b2440] pt-4 text-[11px] text-[#687493]"><span>Data Vision · Revision {{ dashboard?.revision ?? 1 }}</span><div class="flex items-center gap-4"><button class="flex items-center gap-1.5 hover:text-[#dce4f7]"><Share2 :size="13" />分享</button></div></footer>
      </div>
    </main>

    <aside v-if="editing && selectedPanel" class="no-print fixed inset-y-0 right-0 z-30 w-[390px] overflow-y-auto border-l border-[#202945] bg-[#0b1020] px-5 py-6 shadow-2xl"><div class="flex items-center justify-between"><div><div class="text-xs uppercase tracking-[0.16em] text-[#687493]">面板配置</div><h2 class="mt-1 text-lg font-semibold text-[#f4f5ff]">{{ selectedPanel.title }}</h2></div><button class="text-[#8893b3]" @click="selectedPanelUid = ''">×</button></div><label class="mt-6 block text-xs text-[#8893b3]">标题<input v-model="selectedPanel.title" class="field mt-2" /></label><label class="mt-4 block text-xs text-[#8893b3]">图表类型<select :value="selectedPanel.type" class="field mt-2" @change="changeType(selectedPanel, $event)"><option value="timeseries">折线图</option><option value="bar">柱状图</option><option value="pie">饼图</option><option value="stat">指标卡</option><option value="gauge">仪表盘</option><option value="table">表格</option></select></label><label class="mt-4 block text-xs text-[#8893b3]">查询模式<select :value="queryMode(selectedPanel)" class="field mt-2" @change="changeMode(selectedPanel, $event)"><option value="none">未配置</option><option value="sql">SQL</option><option value="http">HTTP API</option><option value="pipeline">跨数据源编排</option></select></label>
      <template v-if="queryMode(selectedPanel) === 'sql'"><label class="mt-4 block text-xs text-[#8893b3]">数据源<select v-model="selectedQuery.datasourceUid" class="field mt-2"><option v-for="source in sourceOptions('db')" :key="source.uid" :value="source.uid">{{ source.name }} · {{ source.type }}</option></select></label><label class="mt-4 block text-xs text-[#8893b3]">SQL<SqlEditor v-model="selectedQuery.sql" class="mt-2" :dialect="selectedSqlDialect" :min-height="220" placeholder="SELECT ... WHERE created_at >= :start" /></label><label class="mt-4 block text-xs text-[#8893b3]">命名参数 JSON<textarea v-model="sqlParamsText" class="field code mt-2" rows="3" placeholder="{ &quot;start&quot;: &quot;2026-01-01&quot; }" /></label><button class="action mt-3" @click="runPanel(selectedPanel)">执行查询预览</button></template>
      <template v-else-if="queryMode(selectedPanel) === 'http'"><label class="mt-4 block text-xs text-[#8893b3]">HTTP 数据源<select v-model="selectedQuery.datasourceUid" class="field mt-2"><option v-for="source in sourceOptions('http')" :key="source.uid" :value="source.uid">{{ source.name }}</option></select></label><div class="mt-4 grid grid-cols-3 gap-2"><label class="text-xs text-[#8893b3]">方法<select v-model="selectedRequest.method" class="field mt-2"><option>GET</option><option>POST</option></select></label><label class="col-span-2 text-xs text-[#8893b3]">路径<input v-model="selectedRequest.path" class="field mt-2" placeholder="/metrics" /></label></div><label class="mt-4 block text-xs text-[#8893b3]">查询参数 JSON<textarea v-model="requestParamsText" class="field code mt-2" rows="3" placeholder="{ &quot;page&quot;: 1 }" /></label><label v-if="selectedRequest.method !== 'GET'" class="mt-4 block text-xs text-[#8893b3]">请求体 JSON<textarea v-model="requestBodyText" class="field code mt-2" rows="4" placeholder="{ &quot;limit&quot;: 100 }" /></label><label class="mt-4 block text-xs text-[#8893b3]">数据行路径<input v-model="selectedRequest.rowsPath" class="field mt-2" placeholder="data.items" /></label><label class="mt-4 block text-xs text-[#8893b3]">字段映射 JSON<textarea v-model="fieldMapText" class="field code mt-2" rows="5" placeholder="{ &quot;name&quot;: &quot;name&quot;, &quot;value&quot;: &quot;value&quot; }" /></label><button class="action mt-3" @click="runPanel(selectedPanel)">执行请求预览</button></template>
      <template v-else-if="queryMode(selectedPanel) === 'pipeline'"><label class="mt-4 block text-xs text-[#8893b3]">Pipeline JSON<textarea :value="ensurePipelineText(selectedPanel)" class="field code mt-2" rows="18" spellcheck="false" @input="changePipeline(selectedPanel, $event)" /></label><button class="action mt-3" @click="runPanel(selectedPanel)">执行编排预览</button></template>
      <div v-if="selectedResult?.columns.length" class="mt-6"><div class="text-xs uppercase tracking-[0.16em] text-[#687493]">字段映射</div><label class="mt-3 block text-xs text-[#8893b3]">维度字段<select :value="selectedPanel.visualization?.xField ?? ''" class="field mt-2" @change="changeField(selectedPanel, 'xField', $event)"><option value="">自动选择</option><option v-for="column in selectedResult.columns" :key="column.name" :value="column.name">{{ column.name }}</option></select></label><label class="mt-3 block text-xs text-[#8893b3]">指标字段<select :value="selectedPanel.visualization?.yField ?? ''" class="field mt-2" @change="changeField(selectedPanel, 'yField', $event)"><option value="">自动选择</option><option v-for="column in selectedResult.columns" :key="column.name" :value="column.name">{{ column.name }}</option></select></label></div><div v-if="selectedPanel.queryConfig?.mode === 'none'" class="mt-6 rounded-xl border border-dashed border-[#2b3554] px-4 py-5 text-center text-xs leading-5 text-[#687493]">选择 SQL、HTTP API 或 Pipeline 开始配置数据。</div></aside>
  </div>
</template>

<style scoped>
 .field { width: 100%; border: 1px solid var(--app-border-strong); border-radius: 0.65rem; background: var(--app-surface); color: var(--app-text); padding: 0.55rem 0.75rem; line-height: 1.35; outline: none; }
 .field:focus { border-color: var(--app-primary); }
.code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; line-height: 1.5; resize: vertical; }
 .action { width: 100%; border-radius: 0.65rem; background: var(--app-primary); padding: 0.65rem; font-size: 0.75rem; font-weight: 600; color: white; }
@media (max-width: 900px) { main { margin-left: 0 !important; } }
@media print { .grid-shell { height: auto !important; } }
</style>
