<script setup lang="ts">
// 工作负载列表：按名称、健康和指标异常筛选 Deployment，点击记录进入详情页。
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import StatePanel from '@/components/StatePanel.vue'
import { useUiStore } from '@/stores/ui'
import type { Page, Workload } from '@/types'
import { formatMetric, healthLabels, relativeTime, sourceLabels } from '@/utils'

const ui = useUiStore()
const router = useRouter()
const route = useRoute()

const loading = ref(true)
const error = ref('')
const data = ref<Page<Workload> | null>(null)

const q = ref(typeof route.query.q === 'string' ? route.query.q : '')
const health = ref('')
const metricIssue = ref(false)
const page = ref(1)

// 将搜索、健康状态和指标异常条件提交给列表 API。
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.workloads({
      namespace: ui.namespace || undefined,
      q: q.value,
      health: health.value || undefined,
      metricIssue: metricIssue.value,
      page: page.value,
      pageSize: 20,
    })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取工作负载'
  } finally {
    loading.value = false
  }
}

// 对命名空间和名称编码后跳转到工作负载详情。
function open(row: Workload) {
  router.push(`/workloads/${encodeURIComponent(row.namespace)}/${encodeURIComponent(row.name)}`)
}

onMounted(load)
watch(
  () => [ui.namespace, ui.refreshKey, q.value, health.value, metricIssue.value, page.value],
  load,
)
</script>

<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">WORKLOAD INVENTORY</div>
      <h1>工作负载</h1>
      <p>查看 Deployment 的健康状态、指标与伸缩控制来源</p>
    </div>
    <span class="muted">{{ data?.total ?? 0 }} 个 Deployment</span>
  </div>
  <section class="card table-card">
    <div class="toolbar">
      <el-input
        v-model="q"
        placeholder="搜索工作负载名称"
        clearable
      >
        <template #prefix>⌕</template>
      </el-input>
      <el-select
        v-model="health"
        placeholder="健康状态"
      >
        <el-option
          label="全部状态"
          value=""
        />
        <el-option
          label="健康"
          value="healthy"
        />
        <el-option
          label="需关注"
          value="warning"
        />
        <el-option
          label="异常"
          value="critical"
        />
      </el-select>
      <el-checkbox v-model="metricIssue">只看指标异常</el-checkbox>
      <span
        class="muted"
        style="margin-left: auto"
      >
        指标缺失以 — 显示
      </span>
    </div>
    <StatePanel
      :loading="loading"
      :error="error"
      :empty="data?.items.length === 0"
      @retry="load"
    >
      <el-table
        :data="data?.items || []"
        stripe
        @row-click="open"
      >
        <el-table-column
          label="工作负载"
          min-width="190"
        >
          <template #default="{ row }">
            <span class="name-cell link">{{ row.name }}</span>
            <span class="subline">{{ row.namespace }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="健康状态"
          width="105"
        >
          <template #default="{ row }">
            <span
              class="badge"
              :class="row.health"
            >
              {{ healthLabels[row.health as keyof typeof healthLabels] }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="副本"
          width="95"
        >
          <template #default="{ row }">
            <strong class="mono">{{ row.readyReplicas }}/{{ row.desiredReplicas }}</strong>
            <span class="subline">Ready / 期望</span>
          </template>
        </el-table-column>
        <el-table-column
          label="CPU"
          width="105"
        >
          <template #default="{ row }">
            <span :class="row.cpuUtilization.quality !== 'ok' ? 'metric-missing' : 'mono'">
              {{ formatMetric(row.cpuUtilization) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="内存"
          width="115"
        >
          <template #default="{ row }">
            <span class="mono">{{ formatMetric(row.memoryWorkingSet) }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="RPS"
          width="112"
        >
          <template #default="{ row }">
            <span :class="row.rps.quality !== 'ok' ? 'metric-missing' : 'mono'">
              {{ formatMetric(row.rps) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="控制来源"
          width="120"
        >
          <template #default="{ row }">
            <span class="source-chip">
              {{ sourceLabels[row.controllerSource as keyof typeof sourceLabels] }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="最近伸缩"
          width="100"
        >
          <template #default="{ row }">{{ relativeTime(row.lastScaleAt) }}</template>
        </el-table-column>
        <el-table-column
          label=""
          width="38"
        >
          <template #default>›</template>
        </el-table-column>
      </el-table>
      <div class="table-footer">
        <el-pagination
          v-model:current-page="page"
          small
          background
          layout="prev, pager, next"
          :page-size="20"
          :total="data?.total || 0"
        />
      </div>
    </StatePanel>
  </section>
</template>
