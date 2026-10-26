<script setup lang="ts">
// 操作详情抽屉：展示副本决策过程、安全检查和失败原因，由父页面提供数据。
import { computed } from 'vue'
import type { Operation } from '@/types'
import { formatTime, statusLabels } from '@/utils'

const props = defineProps<{ detail: Operation | null }>()
const visible = defineModel<boolean>({ required: true })
const detail = computed(() => props.detail)

const detailRows = computed(() => {
  if (!detail.value) return []
  return [
    { label: '操作 ID', value: detail.value.id },
    { label: '工作负载', value: `${detail.value.namespace}/${detail.value.workloadName}` },
    { label: '类型', value: detail.value.type },
    { label: '状态', value: statusLabels[detail.value.status] },
    { label: '指标值', value: detail.value.metricValue ?? '—' },
    { label: '目标值', value: detail.value.targetValue ?? '—' },
    { label: '原副本', value: detail.value.oldReplicas ?? '—' },
    { label: '候选副本', value: detail.value.candidateReplicas ?? '—' },
    { label: '期望副本', value: detail.value.desiredReplicas ?? '—' },
    { label: '实际副本', value: detail.value.actualReplicas ?? '—' },
    { label: '开始时间', value: formatTime(detail.value.createdAt) },
    { label: '结束时间', value: formatTime(detail.value.finishedAt) },
  ]
})
</script>

<template>
  <el-drawer
    v-model="visible"
    title="决策详情"
    size="460px"
  >
    <template v-if="detail">
      <div
        v-for="item in detailRows"
        :key="item.label"
        class="drawer-row"
      >
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </div>
      <h4>安全检查</h4>
      <div
        v-for="check in detail.checks"
        :key="check.code"
        class="drawer-row"
      >
        <span>{{ check.code }}</span>
        <strong :style="{ color: check.passed ? 'var(--green)' : 'var(--red)' }">
          {{ check.reason }}
        </strong>
      </div>
      <div
        v-if="detail.errorMessage"
        class="callout"
      >
        {{ detail.errorMessage }}
      </div>
    </template>
  </el-drawer>
</template>
