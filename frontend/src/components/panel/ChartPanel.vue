<script setup lang="ts">
import { computed } from 'vue'
import type { EChartsOption } from 'echarts'
import { BarChart, GaugeChart, LineChart, PieChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TitleComponent, TooltipComponent } from 'echarts/components'
import { use } from 'echarts/core'
import { SVGRenderer } from 'echarts/renderers'
import VChart from 'vue-echarts'
import type { Panel, QueryResult } from '@/types/dashboard'
import { useTheme } from '@/composables/useTheme'

use([SVGRenderer, LineChart, BarChart, PieChart, GaugeChart, GridComponent, LegendComponent, TitleComponent, TooltipComponent])

const props = defineProps<{ panel: Panel; result?: QueryResult | null; loading?: boolean; error?: string; editable?: boolean }>()
const emit = defineEmits<{ edit: []; remove: []; duplicate: [] }>()
const palette = ['#7c83fd', '#3dd9b4', '#f7b955', '#f4779f', '#57b6f7']
const { theme } = useTheme()

const chartTheme = computed(() => theme.value === 'dark'
  ? { axis: '#2b3554', grid: '#202945', label: '#8893b3', detail: '#eef2ff' }
  : { axis: '#cbd3e3', grid: '#e5e9f1', label: '#687493', detail: '#26324d' })

const columns = computed(() => props.result?.columns ?? [])
const rows = computed(() => props.result?.rows ?? [])
const visualization = computed(() => props.panel.visualization ?? {})
const numericColumn = computed(() => columns.value.find((column) => column.type === 'number')?.name ?? columns.value[1]?.name ?? columns.value[0]?.name ?? '')
const labelColumn = computed(() => visualization.value.xField ?? visualization.value.nameField ?? columns.value.find((column) => column.type !== 'number')?.name ?? columns.value[0]?.name ?? '')
const valueColumn = computed(() => visualization.value.yField ?? visualization.value.valueField ?? numericColumn.value)
const statValue = computed(() => {
  const value = rows.value[0]?.[valueColumn.value]
  return value === undefined || value === null ? '—' : formatValue(value)
})

const option = computed<EChartsOption>(() => {
  const labels = rows.value.map((row) => String(row[labelColumn.value] ?? ''))
  const values = rows.value.map((row) => Number(row[valueColumn.value] ?? 0))
  const colors = chartTheme.value
  if (props.panel.type === 'bar') {
    return { color: palette, tooltip: { trigger: 'axis' }, grid: { top: 18, right: 12, bottom: 28, left: 42 }, xAxis: { type: 'category', data: labels, axisLine: { lineStyle: { color: colors.axis } }, axisLabel: { color: colors.label } }, yAxis: { type: 'value', splitLine: { lineStyle: { color: colors.grid } }, axisLabel: { color: colors.label } }, series: [{ type: 'bar', barWidth: 18, itemStyle: { borderRadius: [5, 5, 0, 0] }, data: values }] }
  }
  if (props.panel.type === 'pie') {
    return { color: palette, tooltip: { trigger: 'item' }, legend: { bottom: 0, textStyle: { color: colors.label } }, series: [{ type: 'pie', radius: ['45%', '72%'], center: ['50%', '45%'], data: rows.value.map((row) => ({ value: Number(row[valueColumn.value] ?? 0), name: String(row[labelColumn.value] ?? '') })), label: { show: false } }] }
  }
  if (props.panel.type === 'gauge') {
    const value = Number(rows.value[0]?.[valueColumn.value] ?? 0)
    return { series: [{ type: 'gauge', startAngle: 210, endAngle: -30, min: 0, max: 100, progress: { show: true, width: 12, itemStyle: { color: '#3dd9b4' } }, axisLine: { lineStyle: { width: 12, color: [[1, colors.grid]] } }, axisTick: { show: false }, splitLine: { show: false }, axisLabel: { show: false }, pointer: { show: false }, anchor: { show: false }, detail: { valueAnimation: true, offsetCenter: [0, '-5%'], color: colors.detail, fontSize: 30, formatter: '{value}%' }, data: [{ value, name: props.panel.title }] }] }
  }
  return { color: [palette[0], palette[1]], tooltip: { trigger: 'axis' }, legend: { top: 0, right: 0, textStyle: { color: colors.label } }, grid: { top: 34, right: 12, bottom: 28, left: 42 }, xAxis: { type: 'category', boundaryGap: false, data: labels, axisLine: { lineStyle: { color: colors.axis } }, axisLabel: { color: colors.label } }, yAxis: { type: 'value', splitLine: { lineStyle: { color: colors.grid } }, axisLabel: { color: colors.label } }, series: [{ name: valueColumn.value, type: 'line', smooth: true, showSymbol: false, areaStyle: { color: 'rgba(124, 131, 253, 0.12)' }, lineStyle: { width: 3 }, data: values }] }
})

function formatValue(value: unknown) {
  if (typeof value === 'number') return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(value)
  return String(value)
}
</script>

<template>
  <div class="chart-panel flex h-full min-h-[270px] flex-col overflow-hidden rounded-2xl border border-[#202945] bg-[#11182b] shadow-[0_16px_40px_rgba(0,0,0,0.14)]">
    <div class="flex items-center justify-between px-5 pt-4">
      <div>
        <h3 class="text-sm font-semibold tracking-wide text-[#eef2ff]">{{ panel.title }}</h3>
        <p class="mt-1 text-[11px] text-[#687493]">{{ result ? `${result.meta.rowCount} 行 · ${result.meta.durationMs}ms` : '未配置数据查询' }}</p>
      </div>
      <div v-if="editable" class="no-print flex items-center gap-1" @pointerdown.stop>
        <button class="rounded-lg px-2 py-1 text-[11px] text-[#8893b3] hover:bg-[#1a2340] hover:text-white" @click="emit('edit')">配置</button>
        <button class="rounded-lg px-2 py-1 text-[11px] text-[#8893b3] hover:bg-[#1a2340] hover:text-white" @click="emit('duplicate')">复制</button>
        <button class="rounded-lg px-2 py-1 text-[11px] text-[#f4779f] hover:bg-[#321d35]" @click="emit('remove')">删除</button>
      </div>
    </div>

    <div v-if="loading" class="flex flex-1 items-center justify-center text-xs text-[#8893b3]">正在查询数据…</div>
    <div v-else-if="error" class="flex flex-1 items-center justify-center px-5 text-center text-xs text-[#f7b955]">{{ error }}</div>
    <div v-else-if="!result || !result.rows.length" class="flex flex-1 items-center justify-center text-xs text-[#687493]">暂无数据</div>
    <div v-else-if="panel.type === 'stat'" class="flex flex-1 flex-col justify-center px-5 pb-4"><span class="text-4xl font-semibold tracking-tight text-[#eef2ff]">{{ statValue }}</span><div class="mt-8 h-1.5 overflow-hidden rounded-full bg-[#202945]"><div class="h-full w-3/4 rounded-full bg-gradient-to-r from-[#6670ee] to-[#3dd9b4]"></div></div><div class="mt-3 flex justify-between text-[11px] text-[#687493]"><span>{{ valueColumn || '指标' }}</span><span>当前查询结果</span></div></div>
    <div v-else-if="panel.type === 'table'" class="min-h-0 flex-1 overflow-auto px-4 pb-4"><table class="w-full text-left text-xs"><thead class="text-[#8893b3]"><tr><th v-for="column in columns" :key="column.name" class="border-b border-[#202945] px-2 py-2 font-medium">{{ column.name }}</th></tr></thead><tbody><tr v-for="(row, index) in rows.slice(0, 100)" :key="index" class="border-b border-[#18223b] text-[#c5ccef]"><td v-for="column in columns" :key="column.name" class="px-2 py-2">{{ row[column.name] ?? '—' }}</td></tr></tbody></table></div>
    <VChart v-else class="min-h-0 flex-1 px-2 pb-3" :option="option" autoresize />
  </div>
</template>
