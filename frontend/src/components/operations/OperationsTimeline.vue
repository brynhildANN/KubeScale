<script setup lang="ts">
// 操作时间线：优先展示未通过检查的原因，其次展示错误或副本变化，点击后通知父页面。
import type { Operation } from '@/types'
import { formatTime, statusLabels } from '@/utils'

defineProps<{ items: Operation[] }>()
const emit = defineEmits<{ select: [operation: Operation] }>()

// 优先返回未通过检查的原因，再返回错误信息或副本变化。
function operationSummary(operation: Operation) {
  return (
    operation.checks.find((check) => !check.passed)?.reason ||
    operation.errorMessage ||
    `副本 ${operation.oldReplicas ?? '—'} → ${operation.desiredReplicas ?? '—'}`
  )
}
</script>

<template>
  <div class="timeline">
    <div
      v-for="op in items"
      :key="op.id"
      class="timeline-item"
      style="cursor: pointer"
      @click="emit('select', op)"
    >
      <strong>
        {{ op.workloadName }} · {{ op.type }}
        <span
          class="badge"
          :class="op.status"
        >
          {{ statusLabels[op.status] }}
        </span>
      </strong>
      <p>{{ operationSummary(op) }}</p>
      <small>{{ formatTime(op.createdAt) }} · {{ op.namespace }}</small>
    </div>
  </div>
</template>
