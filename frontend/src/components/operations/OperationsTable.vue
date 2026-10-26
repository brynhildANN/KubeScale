<script setup lang="ts">
// 操作表格：展示时间、工作负载、副本变化和状态，点击后向父页面发送选中事件。
import type { Operation } from '@/types'
import { formatTime, statusLabels } from '@/utils'

defineProps<{ items: Operation[] }>()
const emit = defineEmits<{ select: [operation: Operation] }>()
</script>

<template>
  <el-table
    :data="items"
    stripe
    @row-click="emit('select', $event)"
  >
    <el-table-column
      label="时间"
      width="155"
    >
      <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
    </el-table-column>
    <el-table-column
      label="工作负载"
      min-width="170"
    >
      <template #default="{ row }">
        <strong class="name-cell link">{{ row.workloadName }}</strong>
        <span class="subline">{{ row.namespace }}</span>
      </template>
    </el-table-column>
    <el-table-column
      prop="type"
      label="操作类型"
      width="120"
    />
    <el-table-column
      label="副本变化"
      width="130"
    >
      <template #default="{ row }">
        <span class="mono">{{ row.oldReplicas ?? '—' }} → {{ row.desiredReplicas ?? '—' }}</span>
      </template>
    </el-table-column>
    <el-table-column
      label="状态"
      width="95"
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
      prop="source"
      label="来源"
      width="105"
    />
    <el-table-column
      label=""
      width="38"
    >
      <template #default>›</template>
    </el-table-column>
  </el-table>
</template>
