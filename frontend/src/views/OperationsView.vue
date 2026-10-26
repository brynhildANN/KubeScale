<script setup lang="ts">
// 操作记录页：按状态分页查询历史，在表格与时间线间切换，并请求选中记录的详情。
import { onMounted, ref, watch } from 'vue'
import { api } from '@/api'
import StatePanel from '@/components/StatePanel.vue'
import OperationDetailDrawer from '@/components/operations/OperationDetailDrawer.vue'
import OperationsTable from '@/components/operations/OperationsTable.vue'
import OperationsTimeline from '@/components/operations/OperationsTimeline.vue'
import { useUiStore } from '@/stores/ui'
import type { Operation, Page } from '@/types'

const ui = useUiStore()

const loading = ref(true)
const error = ref('')
const data = ref<Page<Operation> | null>(null)

const status = ref('')
const page = ref(1)
const view = ref<'table' | 'timeline'>('table')

const detail = ref<Operation | null>(null)
const drawer = ref(false)

// 按命名空间、操作状态和页码加载记录。
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.operations({
      namespace: ui.namespace || undefined,
      status: status.value || undefined,
      page: page.value,
      pageSize: 20,
    })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取操作记录'
  } finally {
    loading.value = false
  }
}

// 读取选中记录的详情，失败时在页面展示错误。
async function open(row: Operation) {
  try {
    detail.value = await api.operation(row.id)
    drawer.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取操作详情'
  }
}

onMounted(load)
watch(() => [ui.namespace, ui.refreshKey, status.value, page.value], load)
</script>

<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">DECISION HISTORY</div>
      <h1>操作记录</h1>
      <p>追踪每次伸缩、跳过决策和策略变更</p>
    </div>
    <el-radio-group
      v-model="view"
      size="small"
    >
      <el-radio-button value="table">表格</el-radio-button>
      <el-radio-button value="timeline">时间线</el-radio-button>
    </el-radio-group>
  </div>
  <section class="card table-card">
    <div class="toolbar">
      <el-select
        v-model="status"
        placeholder="执行状态"
      >
        <el-option
          label="全部状态"
          value=""
        />
        <el-option
          label="成功"
          value="succeeded"
        />
        <el-option
          label="失败"
          value="failed"
        />
        <el-option
          label="已跳过"
          value="skipped"
        />
      </el-select>
      <span class="muted">点击记录查看指标值、安全检查及执行结果</span>
    </div>
    <StatePanel
      :loading="loading"
      :error="error"
      :empty="data?.items.length === 0"
      @retry="load"
    >
      <OperationsTable
        v-if="view === 'table'"
        :items="data?.items || []"
        @select="open"
      />
      <OperationsTimeline
        v-else
        :items="data?.items || []"
        @select="open"
      />
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
  <OperationDetailDrawer
    v-model="drawer"
    :detail="detail"
  />
</template>
