<script setup lang="ts">
// 设置页：展示集群与服务连接状态，并提供界面偏好设置。
import { onMounted, ref, watch } from 'vue'
import { api, isMock } from '@/api'
import StatePanel from '@/components/StatePanel.vue'
import { useUiStore } from '@/stores/ui'
import type { Settings } from '@/types'
import { formatTime } from '@/utils'

const ui = useUiStore(),
  loading = ref(true),
  error = ref(''),
  data = ref<Settings | null>(null)
// 查询后端系统状态，用于展示集群、指标服务与控制器连接情况。
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.settings()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取系统状态'
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => ui.refreshKey, load)
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">SYSTEM CONFIGURATION</div>
      <h1>系统设置</h1>
      <p>连接与控制器信息为只读状态；界面偏好保存在当前浏览器</p>
    </div>
    <el-button @click="load">刷新状态</el-button>
  </div>
  <StatePanel
    :loading="loading"
    :error="error"
    @retry="load"
  >
    <div
      v-if="data"
      class="settings-grid"
    >
      <section class="card settings-card">
        <h3>Kubernetes 集群</h3>
        <div
          v-for="[label, value] in [
            ['集群名称', data.clusterName],
            ['Kubernetes 版本', data.kubernetesVersion],
            ['节点数量', data.nodeCount],
            ['API 连接', data.kubernetesConnected ? '已连接' : '不可用'],
          ]"
          :key="label"
          class="info-row"
        >
          <span>{{ label }}</span>
          <strong>{{ value }}</strong>
        </div>
      </section>
      <section class="card settings-card">
        <h3>Prometheus</h3>
        <div
          v-for="[label, value] in [
            ['连接状态', data.prometheusConnected ? '已连接' : '不可用'],
            ['服务地址', data.prometheusAddress],
            ['采样周期', `${data.samplingIntervalSeconds} 秒`],
          ]"
          :key="label"
          class="info-row"
        >
          <span>{{ label }}</span>
          <strong>{{ value }}</strong>
        </div>
      </section>
      <section class="card settings-card">
        <h3>Alertmanager</h3>
        <div
          v-for="[label, value] in [
            ['连接状态', data.alertmanagerConnected ? '已连接' : '不可用'],
            ['服务地址', data.alertmanagerAddress],
          ]"
          :key="label"
          class="info-row"
        >
          <span>{{ label }}</span>
          <strong>{{ value }}</strong>
        </div>
      </section>
      <section class="card settings-card">
        <h3>伸缩控制器</h3>
        <div
          v-for="[label, value] in [
            ['运行状态', data.controllerConnected ? '运行中' : '不可用'],
            ['最近心跳', formatTime(data.controllerLastHeartbeat)],
            ['数据模式', isMock ? 'Mock 演示' : '真实 API'],
          ]"
          :key="label"
          class="info-row"
        >
          <span>{{ label }}</span>
          <strong>{{ value }}</strong>
        </div>
      </section>
      <section class="card settings-card full-width">
        <h3>界面偏好</h3>
        <div class="info-row">
          <span>深色模式</span>
          <el-switch v-model="ui.dark" />
        </div>
        <div class="info-row">
          <span>默认图表时间范围</span>
          <el-select
            v-model="ui.range"
            style="width: 190px"
          >
            <el-option
              label="最近 15 分钟"
              value="15m"
            />
            <el-option
              label="最近 1 小时"
              value="1h"
            />
            <el-option
              label="最近 6 小时"
              value="6h"
            />
            <el-option
              label="最近 24 小时"
              value="24h"
            />
            <el-option
              label="自定义（当前会话）"
              value="custom"
            />
          </el-select>
        </div>
      </section>
    </div>
  </StatePanel>
</template>
