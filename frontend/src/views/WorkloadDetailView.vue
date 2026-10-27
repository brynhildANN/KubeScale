<script setup lang="ts">
// 工作负载详情：加载指标曲线、Pod、策略和事件，提供手动伸缩与策略暂停入口。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import MetricChart from '@/components/MetricChart.vue'
import StatePanel from '@/components/StatePanel.vue'
import AlertDetailDrawer from '@/components/alerts/AlertDetailDrawer.vue'
import OperationDetailDrawer from '@/components/operations/OperationDetailDrawer.vue'
import { useUiStore } from '@/stores/ui'
import type { Alert, Operation, Page, Pod, Policy, SeriesResponse, Workload } from '@/types'
import { formatMetric, formatTime, healthLabels, sourceLabels, statusLabels } from '@/utils'

const route = useRoute(),
  router = useRouter(),
  ui = useUiStore()
const namespace = String(route.params.namespace),
  name = String(route.params.name)
const loading = ref(true),
  error = ref(''),
  workload = ref<Workload | null>(null),
  policy = ref<Policy | null>(null),
  pods = ref<Page<Pod> | null>(null),
  events = ref<Page<Operation> | null>(null),
  series = ref<SeriesResponse | null>(null)
const alerts = ref<Alert[]>([])
const alertDetail = ref<Alert | null>(null)
const alertOpen = ref(false)
function openAlert(item: Alert) {
  alertDetail.value = item
  alertOpen.value = true
}
const tab = ref('overview'),
  podDetail = ref<Pod | null>(null),
  podDrawer = ref(false),
  scaleOpen = ref(false),
  scaleCount = ref(1),
  scaling = ref(false)
const decision = ref<Operation | null>(null)
const decisionOpen = ref(false)
const customRange = ref<[Date, Date] | null>(null)
const timeRanges = [
  ['15m', '15 分钟'],
  ['1h', '1 小时'],
  ['6h', '6 小时'],
  ['24h', '24 小时'],
  ['custom', '自定义'],
]
const canScale = computed(
  () =>
    workload.value &&
    !['hpa', 'external'].includes(workload.value.controllerSource) &&
    policy.value?.status !== 'enabled',
)
// 并行查询工作负载、策略、事件、Pod 和指标曲线，再组装当前详情。
async function load() {
  loading.value = true
  error.value = ''
  try {
    const to = ui.range === 'custom' && customRange.value ? customRange.value[1] : new Date()
    const duration =
      ui.range === '15m'
        ? 900000
        : ui.range === '6h'
          ? 21600000
          : ui.range === '24h'
            ? 86400000
            : 3600000
    const from =
      ui.range === 'custom' && customRange.value
        ? customRange.value[0]
        : new Date(to.getTime() - duration)
    const [w, ev, ps, s, alertPage] = await Promise.all([
      api.workload(namespace, name),
      api.workloadEvents(namespace, name),
      api.pods(namespace, name),
      api.series(
        [
          'cpu_utilization',
          'memory_working_set',
          'rps',
          'latency_p95',
          'http_error_ratio',
          'desired_replicas',
          'ready_replicas',
        ],
        from.toISOString(),
        to.toISOString(),
        namespace,
        name,
      ),
      api.alerts({ namespace, workload: name, status: 'firing' }),
    ])
    alerts.value = alertPage.items
    workload.value = w
    policy.value = w.policyId ? await api.policy(w.policyId) : null
    events.value = ev
    pods.value = ps
    series.value = s
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取工作负载详情'
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => [ui.refreshKey, ui.range, customRange.value], load)
// 按指标 ID 提取曲线，未加载时返回空数组。
function openDecision(event: Operation) {
  decision.value = event
  decisionOpen.value = true
}
function metric(id: string) {
  return series.value?.series.filter((s) => s.metricId === id) ?? []
}
// 以当前期望副本为初始值打开手动伸缩窗口。
function openScale() {
  scaleCount.value = workload.value?.desiredReplicas ?? 1
  scaleOpen.value = true
}
// 确认后提交手动副本调整，并刷新详情。
async function scale() {
  if (!workload.value) return
  if (scaleCount.value < 1) {
    ElMessage.warning('副本数至少为 1')
    return
  }
  scaling.value = true
  try {
    await ElMessageBox.confirm(
      `将 ${name} 调整为 ${scaleCount.value} 个副本？此操作将写入操作记录。`,
      '确认手动伸缩',
      { type: 'warning' },
    )
    await api.scale(namespace, name, scaleCount.value, '控制台手动调整')
    ElMessage.success('伸缩请求已提交')
    scaleOpen.value = false
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e instanceof Error ? e.message : '伸缩失败')
  } finally {
    scaling.value = false
  }
}
// 携带当前版本暂停或启用策略，成功后重新加载。
async function togglePause() {
  if (!policy.value) return
  try {
    await api.setPolicyStatus(
      policy.value.id,
      policy.value.status === 'enabled' ? 'paused' : 'enabled',
      policy.value.version,
    )
    ElMessage.success('策略状态已更新')
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '操作失败')
  }
}
</script>
<template>
  <section
    v-if="alerts.length"
    class="card"
  >
    <h3>活动告警（最近 20 条）</h3>
    <p
      v-for="item in alerts"
      :key="item.id"
    >
      <button
        class="link"
        @click="openAlert(item)"
      >
        {{ item.name }}：{{ item.summary }}
      </button>
    </p>
  </section>
  <AlertDetailDrawer
    v-model="alertOpen"
    :detail="alertDetail"
  />

  <StatePanel
    :loading="loading"
    :error="error"
    @retry="load"
  >
    <template v-if="workload">
      <div class="page-heading">
        <div>
          <router-link
            to="/workloads"
            class="link muted"
          >
            ← 返回工作负载
          </router-link>
        </div>
        <div class="heading-actions">
          <el-date-picker
            v-if="ui.range === 'custom'"
            v-model="customRange"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            :disabled-date="(date: Date) => date.getTime() > Date.now()"
            style="width: 360px"
          />
          <el-select
            v-model="ui.range"
            style="width: 132px"
          >
            <el-option
              v-for="[v, l] in timeRanges"
              :key="v"
              :value="v"
              :label="l"
            />
          </el-select>
        </div>
      </div>
      <section class="card detail-hero">
        <div>
          <div class="eyebrow">{{ workload.namespace }} / DEPLOYMENT</div>
          <h1>
            {{ workload.name }}
            <span
              class="badge"
              :class="workload.health"
            >
              {{ healthLabels[workload.health] }}
            </span>
          </h1>
          <div class="sub">
            控制来源：{{ sourceLabels[workload.controllerSource] }} · 最近更新
            {{ formatTime(workload.observedAt) }}
          </div>
          <div class="detail-stats">
            <div class="detail-stat">
              <strong>{{ workload.desiredReplicas }}</strong>
              <span>期望副本</span>
            </div>
            <div class="detail-stat">
              <strong>{{ workload.readyReplicas }}</strong>
              <span>Ready 副本</span>
            </div>
            <div class="detail-stat">
              <strong>{{ formatMetric(workload.cpuUtilization) }}</strong>
              <span>CPU 利用率</span>
            </div>
            <div class="detail-stat">
              <strong>{{ formatMetric(workload.rps) }}</strong>
              <span>请求量</span>
            </div>
          </div>
        </div>
        <div class="heading-actions">
          <el-button
            v-if="policy"
            @click="togglePause"
          >
            {{ policy.status === 'enabled' ? '暂停策略' : '恢复策略' }}
          </el-button>
          <el-button
            type="primary"
            :disabled="!canScale"
            @click="openScale"
          >
            手动伸缩
          </el-button>
        </div>
      </section>
      <div
        v-if="!canScale"
        class="callout"
      >
        {{
          policy?.status === 'enabled'
            ? '当前自动伸缩运行中，请先暂停策略后手动调整。'
            : '该工作负载由其他控制器管理，不能在此手动伸缩。'
        }}
      </div>
      <el-tabs
        v-model="tab"
        class="detail-tabs"
      >
        <el-tab-pane
          label="概览"
          name="overview"
        >
          <div class="tab-content">
            <div class="info-grid">
              <div
                v-for="[label, value] in [
                  ['CPU 利用率', formatMetric(workload.cpuUtilization)],
                  ['内存工作集', formatMetric(workload.memoryWorkingSet)],
                  ['每秒请求数', formatMetric(workload.rps)],
                  ['Ready 副本', String(workload.readyReplicas)],
                  ['可用副本', String(workload.availableReplicas)],
                  ['策略范围', policy ? `${policy.minReplicas}–${policy.maxReplicas}` : '—'],
                ]"
                :key="label"
                class="card info-box"
              >
                <div class="label">{{ label }}</div>
                <div class="value">{{ value }}</div>
              </div>
            </div>
            <div
              class="chart-grid"
              style="margin-top: 16px"
            >
              <MetricChart
                title="CPU 利用率"
                :series="metric('cpu_utilization')"
                :operations="events?.items"
                :target-value="
                  policy?.metric === 'cpu_utilization' ? policy.targetValue : undefined
                "
              />
              <MetricChart
                title="每秒请求数"
                :target-value="
                  policy?.metric === 'rps'
                    ? policy.targetValue * workload.desiredReplicas
                    : undefined
                "
                :series="metric('rps')"
                :operations="events?.items"
              />
            </div>
            <section class="card">
              <div class="card-header"><h3>近期伸缩与异常</h3></div>
              <div
                v-for="event in events?.items.slice(0, 4)"
                :key="event.id"
                class="list-item"
              >
                <span class="list-icon">{{ event.type.includes('缩容') ? '↘' : '↗' }}</span>
                <div class="item-main">
                  <strong>{{ event.type }} · {{ event.workloadName }}</strong>
                  <small>
                    {{
                      event.checks.find((c) => !c.passed)?.reason ||
                      event.errorMessage ||
                      statusLabels[event.status]
                    }}
                  </small>
                </div>
                <span class="muted">{{ formatTime(event.createdAt) }}</span>
              </div>
              <div
                v-if="!events?.items.length"
                class="state-panel"
              >
                暂无事件
              </div>
            </section>
          </div>
        </el-tab-pane>
        <el-tab-pane
          label="指标"
          name="metrics"
        >
          <div class="tab-content chart-grid">
            <MetricChart
              title="CPU 利用率"
              :target-value="policy?.metric === 'cpu_utilization' ? policy.targetValue : undefined"
              :series="metric('cpu_utilization')"
              :operations="events?.items"
            />
            <MetricChart
              title="内存工作集"
              :series="metric('memory_working_set')"
            />
            <MetricChart
              title="每秒请求数"
              :target-value="
                policy?.metric === 'rps' ? policy.targetValue * workload.desiredReplicas : undefined
              "
              :series="metric('rps')"
              :operations="events?.items"
            />
            <MetricChart
              title="P95 响应时间"
              :series="metric('latency_p95')"
            />
            <MetricChart
              title="HTTP 5xx 错误率"
              :series="metric('http_error_ratio')"
            />
            <MetricChart
              title="副本数（期望 / Ready）"
              :series="
                series?.series.filter(
                  (s) => s.metricId === 'desired_replicas' || s.metricId === 'ready_replicas',
                ) || []
              "
              :operations="events?.items"
              step
            />
          </div>
        </el-tab-pane>
        <el-tab-pane
          label="自动伸缩"
          name="autoscaling"
        >
          <div class="tab-content two-col">
            <section class="card">
              <div class="card-header">
                <h3>伸缩策略</h3>
                <el-button
                  v-if="policy"
                  size="small"
                  @click="router.push('/policies')"
                >
                  前往策略管理
                </el-button>
              </div>
              <div
                v-if="policy"
                class="info-list"
              >
                <div
                  v-for="[label, value] in [
                    ['状态', statusLabels[policy.status]],
                    ['触发指标', policy.metric === 'rps' ? '每秒请求数' : 'CPU 利用率'],
                    [
                      '目标值',
                      policy.metric === 'rps'
                        ? `${policy.targetValue} req/s / Pod`
                        : `${Math.round(policy.targetValue * 100)}%`,
                    ],
                    ['最小 / 最大副本', `${policy.minReplicas} / ${policy.maxReplicas}`],
                    [
                      '扩 / 缩连续周期',
                      `${policy.scaleUpConsecutivePeriods} / ${policy.scaleDownConsecutivePeriods}`,
                    ],
                    ['冷却期', `${policy.cooldownSeconds} 秒`],
                    ['稳定窗口', `${policy.stabilizationSeconds} 秒`],
                  ]"
                  :key="label"
                  class="info-row"
                >
                  <span>{{ label }}</span>
                  <strong>{{ value }}</strong>
                </div>
              </div>
              <div
                v-else
                class="state-panel"
              >
                当前没有 KubeScale 伸缩策略
              </div>
            </section>
            <section class="card">
              <div class="card-header"><h3>最近决策</h3></div>
              <div class="timeline">
                <div
                  v-for="event in events?.items.slice(0, 5)"
                  :key="event.id"
                  class="timeline-item"
                >
                  <strong>{{ event.type }} · {{ statusLabels[event.status] }}</strong>
                  <p>
                    {{
                      event.checks.find((c) => !c.passed)?.reason ||
                      event.errorMessage ||
                      `从 ${event.oldReplicas ?? '—'} 调整至 ${event.desiredReplicas ?? '—'} 个副本`
                    }}
                  </p>
                  <small>{{ formatTime(event.createdAt) }}</small>
                  <el-button
                    link
                    type="primary"
                    @click="openDecision(event)"
                  >
                    查看决策详情
                  </el-button>
                </div>
                <div
                  v-if="!events?.items.length"
                  class="state-panel"
                >
                  暂无决策
                </div>
              </div>
            </section>
          </div>
        </el-tab-pane>
        <el-tab-pane
          label="Pod"
          name="pods"
        >
          <div class="tab-content card table-card">
            <el-table
              :data="pods?.items || []"
              @row-click="
                (row: Pod) => {
                  podDetail = row
                  podDrawer = true
                }
              "
            >
              <el-table-column
                prop="name"
                label="Pod 名称"
                min-width="230"
              />
              <el-table-column
                prop="phase"
                label="状态"
                width="100"
              />
              <el-table-column
                prop="node"
                label="节点"
                width="115"
              />
              <el-table-column
                label="CPU"
                width="110"
              >
                <template #default="{ row }">{{ formatMetric(row.cpu) }}</template>
              </el-table-column>
              <el-table-column
                label="内存"
                width="120"
              >
                <template #default="{ row }">{{ formatMetric(row.memory) }}</template>
              </el-table-column>
              <el-table-column
                prop="restarts"
                label="重启"
                width="75"
              />
              <el-table-column
                prop="ip"
                label="Pod IP"
                width="130"
              />
              <el-table-column
                label="创建时间"
                width="155"
              >
                <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>
        <el-tab-pane
          label="事件"
          name="events"
        >
          <div class="tab-content card">
            <div class="timeline">
              <div
                v-for="event in events?.items"
                :key="event.id"
                class="timeline-item"
              >
                <strong>{{ event.type }} · {{ statusLabels[event.status] }}</strong>
                <p>
                  {{
                    event.errorMessage ||
                    event.checks.find((c) => !c.passed)?.reason ||
                    `期望副本 ${event.desiredReplicas ?? '—'}，实际副本 ${event.actualReplicas ?? '—'}`
                  }}
                </p>
                <small>{{ formatTime(event.createdAt) }} · {{ event.source }}</small>
                <el-button
                  link
                  type="primary"
                  @click="openDecision(event)"
                >
                  查看决策详情
                </el-button>
              </div>
              <div
                v-if="!events?.items.length"
                class="state-panel"
              >
                暂无事件
              </div>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
      <el-dialog
        v-model="scaleOpen"
        title="手动伸缩"
        width="430px"
      >
        <p class="muted">当前 {{ workload.desiredReplicas }} 个副本。调整后会产生一条操作记录。</p>
        <el-input-number
          v-model="scaleCount"
          :min="policy?.minReplicas ?? 1"
          :max="policy?.maxReplicas ?? 1000"
          style="width: 100%"
        />
        <template #footer>
          <el-button @click="scaleOpen = false">取消</el-button>
          <el-button
            type="primary"
            :loading="scaling"
            @click="scale"
          >
            确认调整
          </el-button>
        </template>
      </el-dialog>
      <OperationDetailDrawer
        v-model="decisionOpen"
        :detail="decision"
      />
      <el-drawer
        v-model="podDrawer"
        title="Pod 详情"
        size="420px"
      >
        <template v-if="podDetail">
          <div
            v-for="[label, value] in [
              ['名称', podDetail.name],
              ['状态', podDetail.phase],
              ['节点', podDetail.node],
              ['Pod IP', podDetail.ip],
              ['重启次数', String(podDetail.restarts)],
              ['创建时间', formatTime(podDetail.createdAt)],
            ]"
            :key="label"
            class="drawer-row"
          >
            <span>{{ label }}</span>
            <strong>{{ value }}</strong>
          </div>
          <h4>容器</h4>
          <div
            v-for="container in podDetail.containers"
            :key="container.name"
            class="drawer-row"
          >
            <span>{{ container.name }}</span>
            <strong>{{ container.ready ? 'Ready' : '未就绪' }}</strong>
          </div>
        </template>
      </el-drawer>
    </template>
  </StatePanel>
</template>
