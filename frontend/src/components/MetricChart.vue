<script setup lang="ts">
// 指标图表：管理 ECharts 渲染、单位与缺失值展示，以及窗口尺寸变化和实例销毁。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { MetricSeries, Operation } from '@/types'
import { formatNumber, formatUnit } from '@/utils'

const props = defineProps<{
  title: string
  series: MetricSeries[]
  operations?: Operation[]
  height?: number
  step?: boolean
  targetValue?: number
}>()
echarts.use([
  LineChart,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  CanvasRenderer,
])
const labels: Record<string, string> = {
  cpu_utilization: 'CPU 利用率',
  rps: 'RPS',
  memory_working_set: '内存工作集',
  latency_p95: 'P95 延迟',
  http_error_ratio: '5xx 错误率',
  desired_replicas: '期望副本',
  ready_replicas: 'Ready 副本',
  cluster_cpu_request_ratio: 'CPU 预订率',
  cluster_memory_request_ratio: '内存预订率',
}
const el = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null
let observer: ResizeObserver | null = null
const hasData = computed(() => props.series.some((s) => s.points.some((p) => p.value != null)))
// 将指标曲线转换为图表配置，处理缺失值、单位和事件标记。
function render() {
  if (!el.value || !hasData.value) return
  chart ||= echarts.init(el.value)
  const unit = props.series[0]?.unit ?? 'count'
  const marks = (props.operations ?? [])
    .filter(
      (o) =>
        o.status === 'succeeded' &&
        (o.type.includes('扩容') || o.type.includes('缩容') || o.type.includes('手动')),
    )
    .map((o) => ({
      xAxis: o.createdAt,
      name: o.type,
      label: { show: false },
      lineStyle: {
        color: o.type.includes('扩容') ? '#377df5' : '#e69b3d',
        type: 'dashed' as const,
        width: 1,
      },
    }))
  const target =
    props.targetValue == null
      ? []
      : [
          {
            yAxis: props.targetValue,
            name: unit === 'rps' ? '当前目标 × 当前期望副本' : '当前策略目标',
            label: {
              show: true,
              position: 'insideEndTop' as const,
              formatter: `目标 ${formatUnit(props.targetValue, unit)}`,
            },
            lineStyle: { color: '#e69b3d', type: 'dashed' as const },
          },
        ]
  chart.resize()
  chart.setOption(
    {
      animationDuration: 250,
      color: ['#3a7bf7', '#1db49a', '#e59b39'],
      grid: { left: 56, right: 22, top: 28, bottom: 44 },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        valueFormatter: (v: unknown) => formatUnit(typeof v === 'number' ? v : null, unit),
      },
      legend: { top: 0, right: 0, textStyle: { color: '#8391a7', fontSize: 11 } },
      xAxis: {
        type: 'time',
        boundaryGap: false,
        axisLabel: { color: '#8391a7' },
        axisLine: { lineStyle: { color: '#dce3ed' } },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        min: 0,
        max:
          props.targetValue == null
            ? undefined
            : (extent: { max: number }) => Math.max(extent.max, props.targetValue ?? 0) * 1.1,
        axisLabel: {
          color: '#8391a7',
          formatter: (v: number) =>
            unit === 'ratio'
              ? `${formatNumber(v * 100, 2)}%`
              : unit === 'byte'
                ? `${Math.round(v / 1048576)}`
                : unit === 'second'
                  ? `${Math.round(v * 1000)}`
                  : String(v),
        },
        splitLine: { lineStyle: { color: '#eef1f6' } },
      },
      series: props.series.map((s, i) => ({
        name: labels[s.metricId] ?? s.metricId,
        type: 'line',
        showSymbol: false,
        connectNulls: false,
        step: props.step ? 'end' : false,
        smooth: !props.step,
        lineStyle: { width: 2.5 },
        areaStyle: i === 0 ? { opacity: 0.06 } : undefined,
        data: s.points.map((p) => [p.timestamp, p.value]),
        markLine: i === 0 ? { symbol: 'none', data: [...marks, ...target] } : undefined,
      })),
    },
    true,
  )
}
watch(
  () => [props.series, props.operations, props.targetValue],
  () => requestAnimationFrame(render),
  { deep: true },
)
onMounted(() => {
  render()
  if (el.value) {
    observer = new ResizeObserver(() => chart?.resize())
    observer.observe(el.value)
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  chart?.dispose()
  chart = null
})
</script>
<template>
  <section class="card chart-card">
    <div class="card-header">
      <h3>{{ title }}</h3>
      <span class="muted">
        {{ targetValue == null ? '按本地时区显示' : '目标线使用当前配置；历史参数见决策详情' }}
      </span>
    </div>
    <div
      v-if="!hasData"
      class="chart-empty"
      :style="{ height: (height || 240) + 'px' }"
    >
      暂无可用指标数据
    </div>
    <div
      v-show="hasData"
      ref="el"
      class="chart-canvas"
      :style="{ height: (height || 240) + 'px' }"
      role="img"
      :aria-label="title + '趋势图'"
    />
  </section>
</template>
