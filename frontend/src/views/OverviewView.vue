<script setup lang="ts">
// 集群概览：加载健康摘要与指标曲线，展示需关注的工作负载和近期操作。
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import MetricChart from '@/components/MetricChart.vue'
import StatePanel from '@/components/StatePanel.vue'
import { useUiStore } from '@/stores/ui'
import type { Overview, SeriesResponse } from '@/types'
import { formatTime, healthLabels, relativeTime } from '@/utils'

const ui = useUiStore(),
  router = useRouter()
const loading = ref(true),
  error = ref(''),
  data = ref<Overview | null>(null),
  trends = ref<SeriesResponse | null>(null)
// 并行读取概览数据和指标曲线，按全局命名空间与时间范围刷新展示。
async function load() {
  loading.value = true
  error.value = ''
  try {
    const to = new Date(),
      duration =
        ui.range === '15m'
          ? 900000
          : ui.range === '6h'
            ? 21600000
            : ui.range === '24h'
              ? 86400000
              : 3600000,
      from = new Date(to.getTime() - duration)
    ;[data.value, trends.value] = await Promise.all([
      api.overview(ui.namespace || undefined),
      api.series(
        ['cluster_cpu_request_ratio', 'cluster_memory_request_ratio', 'desired_replicas'],
        from.toISOString(),
        to.toISOString(),
        ui.namespace || undefined,
      ),
    ])
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取概览数据'
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => [ui.namespace, ui.refreshKey, ui.range], load)
const cards = (): Array<[string, string | number, string]> =>
  data.value
    ? [
        ['集群状态', healthLabels[data.value.clusterHealth], 'warning'],
        ['受管工作负载', data.value.managedWorkloads, ''],
        ['当前 Pod 总数', data.value.podCount, ''],
        ['已启用策略', data.value.enabledPolicies, ''],
        ['24h 伸缩次数', data.value.scalingActions24h, ''],
        ['活动告警', data.value.activeAlerts, 'critical'],
      ]
    : []
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">CLUSTER OBSERVABILITY</div>
      <h1>集群概览</h1>
      <p>集中查看工作负载健康、资源预订与伸缩活动</p>
    </div>
    <div class="heading-actions">
      <span class="muted">最近更新 {{ formatTime(data?.observedAt) }}</span>
    </div>
  </div>
  <StatePanel
    :loading="loading"
    :error="error"
    @retry="load"
  >
    <template v-if="data">
      <div class="kpi-grid">
        <div
          v-for="[label, value, tone] in cards()"
          :key="String(label)"
          class="card kpi"
          :class="tone"
        >
          <span class="label">{{ label }}</span>
          <div class="value">{{ value }}</div>
          <span class="accent" />
        </div>
      </div>
      <div class="chart-grid">
        <MetricChart
          title="集群资源预订率"
          :series="trends?.series.filter((s) => s.metricId.includes('request_ratio')) || []"
          :height="252"
        />
        <MetricChart
          title="范围内 Deployment 总副本数"
          :series="trends?.series.filter((s) => s.metricId === 'desired_replicas') || []"
          :operations="data.recentOperations"
          :height="252"
          step
        />
      </div>
      <div class="two-col">
        <section class="card">
          <div class="card-header">
            <h3>需要关注的工作负载</h3>
            <router-link
              class="link muted"
              to="/workloads"
            >
              查看全部 →
            </router-link>
          </div>
          <div
            v-if="!data.attentionWorkloads.length"
            class="state-panel"
          >
            当前没有待关注工作负载
          </div>
          <div
            v-for="item in data.attentionWorkloads"
            :key="item.namespace + item.name"
            class="list-item"
            @click="router.push(`/workloads/${item.namespace}/${item.name}`)"
            style="cursor: pointer"
          >
            <span class="list-icon warn">!</span>
            <div class="item-main">
              <strong>{{ item.name }}</strong>
              <small>{{ item.namespace }} · {{ item.reasonCodes.join(' · ') }}</small>
            </div>
            <span
              class="badge"
              :class="item.health"
            >
              {{ healthLabels[item.health] }}
            </span>
          </div>
        </section>
        <section class="card">
          <div class="card-header">
            <h3>最近操作</h3>
            <router-link
              class="link muted"
              to="/operations"
            >
              查看记录 →
            </router-link>
          </div>
          <div
            v-for="op in data.recentOperations"
            :key="op.id"
            class="list-item"
          >
            <span class="list-icon">{{ op.type.includes('缩容') ? '↘' : '↗' }}</span>
            <div class="item-main">
              <strong>{{ op.workloadName }} · {{ op.type }}</strong>
              <small>
                {{ op.namespace }} ·
                {{
                  op.status === 'skipped'
                    ? '已跳过'
                    : op.status === 'failed'
                      ? '执行失败'
                      : '执行成功'
                }}
              </small>
            </div>
            <span class="time">{{ relativeTime(op.createdAt) }}</span>
          </div>
        </section>
      </div>
    </template>
  </StatePanel>
</template>
