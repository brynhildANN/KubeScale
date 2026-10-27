<script setup lang="ts">
// 操作详情抽屉：展示副本决策过程、安全检查和失败原因，由父页面提供数据。
import { computed } from 'vue'
import type { Operation } from '@/types'
import { formatTime, formatUnit, statusLabels } from '@/utils'

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
    {
      label: '决策指标',
      value:
        detail.value.metricId === 'rps'
          ? '总 RPS'
          : detail.value.metricId === 'cpu_utilization'
            ? 'CPU 利用率'
            : '—',
    },
    {
      label: '指标值',
      value: formatUnit(
        detail.value.metricValue,
        detail.value.metricId === 'rps' ? 'rps' : 'ratio',
      ),
    },
    {
      label: '目标值',
      value:
        formatUnit(detail.value.targetValue, detail.value.metricId === 'rps' ? 'rps' : 'ratio') +
        (detail.value.metricId === 'rps' ? ' / Pod' : ''),
    },
    { label: '指标采样时间', value: formatTime(detail.value.metricSampledAt) },
    { label: '原副本', value: detail.value.oldReplicas ?? '—' },
    { label: '候选副本', value: detail.value.candidateReplicas ?? '—' },
    { label: '期望副本', value: detail.value.desiredReplicas ?? '—' },
    { label: '写后核对副本', value: detail.value.actualReplicas ?? '—' },
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
      <template v-if="detail.policy">
        <h4>决策时的策略配置</h4>
        <div
          v-for="[label, value] in [
            ['策略 / 版本', `${detail.policy.name} / v${detail.policy.version}`],
            ['副本范围', `${detail.policy.minReplicas}–${detail.policy.maxReplicas}`],
            [
              '扩 / 缩连续周期',
              `${detail.policy.scaleUpConsecutivePeriods} / ${detail.policy.scaleDownConsecutivePeriods}`,
            ],
            [
              '冷却 / 稳定窗口',
              `${detail.policy.cooldownSeconds} / ${detail.policy.stabilizationSeconds} 秒`,
            ],
            ['采样周期', `${detail.policy.samplingIntervalSeconds} 秒`],
          ]"
          :key="label"
          class="drawer-row"
        >
          <span>{{ label }}</span>
          <strong>{{ value }}</strong>
        </div>
      </template>
      <p class="form-help">
        期望副本为本次建议，跳过或失败不代表已写入；写后核对来自 Scale.spec，不表示 Pod 已 Ready。
      </p>
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
