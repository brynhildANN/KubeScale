<script setup lang="ts">
// 告警详情抽屉：将选中告警转换为展示字段，并提供关联工作负载跳转。
import { computed } from 'vue'
import type { Alert } from '@/types'
import { formatTime, statusLabels } from '@/utils'

const props = defineProps<{ detail: Alert | null }>()
const visible = defineModel<boolean>({ required: true })
const detail = computed(() => props.detail)

const detailRows = computed(() => {
  if (!detail.value) return []
  return [
    { label: '告警名称', value: detail.value.name },
    { label: '严重程度', value: detail.value.severity },
    { label: '状态', value: statusLabels[detail.value.status] },
    { label: '工作负载', value: `${detail.value.namespace}/${detail.value.workloadName}` },
    { label: '开始时间', value: formatTime(detail.value.startsAt) },
    { label: '恢复时间', value: formatTime(detail.value.endsAt) },
  ]
})
</script>

<template>
  <el-drawer
    v-model="visible"
    title="告警详情"
    size="440px"
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
      <p class="callout">{{ detail.summary }}</p>
      <router-link
        v-if="detail.workloadName"
        class="link"
        :to="`/workloads/${encodeURIComponent(detail.namespace)}/${encodeURIComponent(detail.workloadName)}`"
        @click="visible = false"
      >
        查看工作负载指标与事件 →
      </router-link>
    </template>
  </el-drawer>
</template>
