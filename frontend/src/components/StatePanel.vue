<script setup lang="ts">
// 请求状态容器：统一展示加载、错误和空列表状态，重试时通知父页面。
defineProps<{ loading?: boolean; error?: string | null; empty?: boolean; emptyText?: string }>()
defineEmits<{ retry: [] }>()
</script>
<template>
  <div
    v-if="loading"
    class="state-panel"
    role="status"
  >
    <span class="spinner" />
    正在加载数据…
  </div>
  <div
    v-else-if="error"
    class="state-panel error-panel"
    role="alert"
  >
    <strong>数据加载失败</strong>
    <span>{{ error }}</span>
    <el-button
      size="small"
      @click="$emit('retry')"
    >
      重试
    </el-button>
  </div>
  <div
    v-else-if="empty"
    class="state-panel"
  >
    <span class="empty-symbol">○</span>
    <strong>{{ emptyText || '暂无数据' }}</strong>
    <span>请调整筛选条件或稍后刷新。</span>
  </div>
  <slot v-else />
</template>
