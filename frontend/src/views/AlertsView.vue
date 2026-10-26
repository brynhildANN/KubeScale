<script setup lang="ts">
// 告警中心：加载筛选和分页数据，统计当前页活动/严重告警，查询选中告警的详情。
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '@/api'
import StatePanel from '@/components/StatePanel.vue'
import AlertDetailDrawer from '@/components/alerts/AlertDetailDrawer.vue'
import { useUiStore } from '@/stores/ui'
import type { Alert, Page } from '@/types'
import { formatTime, statusLabels } from '@/utils'

const ui = useUiStore()

const loading = ref(true)
const error = ref('')
const data = ref<Page<Alert> | null>(null)

const status = ref('')
const page = ref(1)

const detail = ref<Alert | null>(null)
const drawer = ref(false)

const active = computed(() => data.value?.items.filter((a) => a.status === 'firing').length ?? 0)
const critical = computed(
  () =>
    data.value?.items.filter((a) => a.status === 'firing' && a.severity === 'critical').length ?? 0,
)

// 按命名空间、告警状态和页码查询列表，并更新加载与错误状态。
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.alerts({
      namespace: ui.namespace || undefined,
      status: status.value || undefined,
      page: page.value,
      pageSize: 20,
    })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取告警'
  } finally {
    loading.value = false
  }
}

// 查询点击告警的完整信息，再打开详情抽屉。
async function open(row: Alert) {
  detail.value = await api.alert(row.id)
  drawer.value = true
}

onMounted(load)
watch(() => [ui.namespace, ui.refreshKey, status.value, page.value], load)
</script>

<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">ALERT CENTER</div>
      <h1>告警中心</h1>
      <p>查看告警触发、恢复及关联工作负载</p>
    </div>
    <span class="muted">由 Alertmanager 提供告警状态</span>
  </div>
  <div
    class="kpi-grid"
    style="grid-template-columns: repeat(3, minmax(0, 1fr))"
  >
    <div class="card kpi critical">
      <span class="label">活动告警</span>
      <div class="value">{{ active }}</div>
      <span class="accent" />
    </div>
    <div class="card kpi warning">
      <span class="label">严重告警</span>
      <div class="value">{{ critical }}</div>
      <span class="accent" />
    </div>
    <div class="card kpi">
      <span class="label">当前列表总数</span>
      <div class="value">{{ data?.total ?? '—' }}</div>
      <span class="accent" />
    </div>
  </div>
  <section class="card table-card">
    <div class="toolbar">
      <el-select
        v-model="status"
        placeholder="告警状态"
      >
        <el-option
          label="全部状态"
          value=""
        />
        <el-option
          label="触发中"
          value="firing"
        />
        <el-option
          label="已恢复"
          value="resolved"
        />
      </el-select>
      <span class="muted">告警静默操作暂未提供</span>
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
          label="告警"
          min-width="170"
        >
          <template #default="{ row }">
            <strong class="name-cell link">{{ row.name }}</strong>
            <span class="subline">{{ row.summary }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="级别"
          width="100"
        >
          <template #default="{ row }">
            <span
              class="badge"
              :class="row.severity === 'critical' ? 'critical' : 'warning'"
            >
              {{ row.severity === 'critical' ? '严重' : '警告' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="工作负载"
          min-width="155"
        >
          <template #default="{ row }">
            {{ row.workloadName }}
            <span class="subline">{{ row.namespace }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="状态"
          width="100"
        >
          <template #default="{ row }">
            <span
              class="badge"
              :class="row.status"
            >
              {{ statusLabels[row.status as keyof typeof statusLabels] }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          label="开始时间"
          width="155"
        >
          <template #default="{ row }">{{ formatTime(row.startsAt) }}</template>
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
  <AlertDetailDrawer
    v-model="drawer"
    :detail="detail"
  />
</template>
