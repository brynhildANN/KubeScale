<script setup lang="ts">
// 策略管理：通过分步表单创建或编辑策略，测试指标查询，并支持启停和删除。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import { validatePolicyDraft } from '@/policy'
import StatePanel from '@/components/StatePanel.vue'
import { useUiStore } from '@/stores/ui'
import type { Page, Policy, PolicyDraft, Workload } from '@/types'
import { formatTime, statusLabels } from '@/utils'

const ui = useUiStore(),
  loading = ref(true),
  error = ref(''),
  data = ref<Page<Policy> | null>(null),
  workloads = ref<Workload[]>([])
const page = ref(1)
const dialog = ref(false),
  step = ref(0),
  editing = ref<Policy | null>(null),
  saving = ref(false),
  queryResult = ref<{ current: number | null; quality: string } | null>(null),
  querying = ref(false)
const initial = (): PolicyDraft => ({
  name: '',
  namespace: '',
  workloadName: '',
  metric: 'cpu_utilization',
  targetValue: 0.7,
  minReplicas: 1,
  maxReplicas: 8,
  scaleUpConsecutivePeriods: 3,
  scaleDownConsecutivePeriods: 5,
  cooldownSeconds: 120,
  stabilizationSeconds: 300,
  samplingIntervalSeconds: 15,
  status: 'disabled',
})
const form = reactive<PolicyDraft>(initial())
const candidates = computed(() => workloads.value.filter((w) => w.namespace === form.namespace))
const selected = computed(() =>
  workloads.value.find((w) => w.namespace === form.namespace && w.name === form.workloadName),
)
// 并行加载策略与工作负载候选列表。
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [p, w] = await Promise.all([
      api.policies({ namespace: ui.namespace || undefined, page: page.value, pageSize: 20 }),
      api.workloads({ pageSize: 100 }),
    ])
    data.value = p
    const candidates = [...w.items]
    for (let next = 2; candidates.length < w.total; next++) {
      const batch = await api.workloads({ page: next, pageSize: 100 })
      if (!batch.items.length) break
      candidates.push(...batch.items)
    }
    workloads.value = candidates
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法获取伸缩策略'
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => [ui.refreshKey, page.value], load)
watch(
  () => ui.namespace,
  () => {
    if (page.value === 1) void load()
    else page.value = 1
  },
)
// 将已有策略或默认值填入表单，打开创建/编辑向导。
function open(item?: Policy) {
  editing.value = item ?? null
  const fields: PolicyDraft = item
    ? {
        name: item.name,
        namespace: item.namespace,
        workloadName: item.workloadName,
        metric: item.metric,
        targetValue: item.targetValue,
        minReplicas: item.minReplicas,
        maxReplicas: item.maxReplicas,
        scaleUpConsecutivePeriods: item.scaleUpConsecutivePeriods,
        scaleDownConsecutivePeriods: item.scaleDownConsecutivePeriods,
        cooldownSeconds: item.cooldownSeconds,
        stabilizationSeconds: item.stabilizationSeconds,
        samplingIntervalSeconds: item.samplingIntervalSeconds,
        status: item.status,
      }
    : initial()
  Object.assign(form, fields)
  step.value = 0
  queryResult.value = null
  dialog.value = true
}
// 结合工作负载控制来源检查表单，返回错误提示或空字符串。
function invalid(): string {
  if (!editing.value && selected.value?.policyId) return '该工作负载已有策略'
  return validatePolicyDraft(form, selected.value?.controllerSource, !!editing.value) || ''
}
// 验证当前向导步骤，通过后进入下一步。
function next() {
  const message =
    step.value === 0
      ? !form.namespace || !form.workloadName
        ? '请选择目标工作负载'
        : !editing.value && selected.value?.policyId
          ? '该工作负载已有策略'
          : !editing.value &&
              selected.value?.controllerSource !== 'unmanaged' &&
              selected.value?.controllerSource !== 'manual'
            ? '工作负载已有伸缩控制者'
            : ''
      : step.value === 1
        ? !form.name.trim()
          ? '请输入策略名称'
          : !form.targetValue ||
              form.targetValue <= 0 ||
              (form.metric === 'cpu_utilization' && form.targetValue > 1)
            ? '目标值不合法'
            : ''
        : invalid()
  if (message) {
    ElMessage.warning(message)
    return
  }
  step.value = Math.min(3, step.value + 1)
}
// 切换指标时重置对应目标值，并清除旧查询结果。
function resetMetricTarget() {
  form.targetValue = form.metric === 'rps' ? 60 : 0.7
  queryResult.value = null
}

// 请求后端验证当前工作负载的指标是否可用。
async function testMetric() {
  if (!form.namespace || !form.workloadName) return
  querying.value = true
  try {
    queryResult.value = await api.validateMetric(
      form.namespace,
      form.workloadName,
      form.metric,
      form.targetValue,
    )
    ElMessage.success('查询完成')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '查询失败')
  } finally {
    querying.value = false
  }
}
// 校验表单后创建或更新策略，编辑时携带版本号。
async function save() {
  const message = invalid()
  if (message) {
    ElMessage.warning(message)
    return
  }
  saving.value = true
  try {
    if (editing.value) await api.updatePolicy(editing.value.id, { ...form }, editing.value.version)
    else await api.createPolicy({ ...form })
    ElMessage.success('策略已保存')
    dialog.value = false
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}
// 在启用与暂停之间切换策略状态，并刷新列表。
async function toggle(item: Policy) {
  try {
    await api.setPolicyStatus(
      item.id,
      item.status === 'enabled' ? 'paused' : 'enabled',
      item.version,
    )
    ElMessage.success('状态已更新')
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '更新失败')
  }
}
// 确认后删除策略，并重新加载列表。
async function remove(item: Policy) {
  try {
    await ElMessageBox.confirm(`删除策略「${item.name}」？`, '删除策略', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.deletePolicy(item.id)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e instanceof Error ? e.message : '删除失败')
  }
}
const metricLabel = (p: Policy) =>
  p.metric === 'rps' ? `${p.targetValue} req/s / Pod` : `${Math.round(p.targetValue * 100)}%`
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">AUTOSCALING POLICIES</div>
      <h1>伸缩策略</h1>
      <p>管理 CPU 与 RPS 目标值策略，每个 Deployment 同时只有一个有效控制者</p>
    </div>
    <el-button
      type="primary"
      @click="open()"
    >
      ＋ 创建策略
    </el-button>
  </div>
  <section class="card table-card">
    <div class="card-header">
      <h3>策略列表</h3>
      <span class="muted">{{ data?.total || 0 }} 条策略</span>
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
      >
        <el-table-column
          label="策略"
          min-width="170"
        >
          <template #default="{ row }">
            <strong class="name-cell">{{ row.name }}</strong>
            <span class="subline">{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="目标工作负载"
          min-width="160"
        >
          <template #default="{ row }">
            <router-link
              class="link"
              :to="`/workloads/${row.namespace}/${row.workloadName}`"
            >
              {{ row.workloadName }}
            </router-link>
            <span class="subline">{{ row.namespace }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="指标 / 目标"
          width="150"
        >
          <template #default="{ row }">
            <span>{{ row.metric === 'rps' ? 'RPS' : 'CPU 利用率' }}</span>
            <span class="subline">{{ metricLabel(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="副本范围"
          width="100"
        >
          <template #default="{ row }">{{ row.minReplicas }} – {{ row.maxReplicas }}</template>
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
          label="更新于"
          width="145"
        >
          <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column
          label="操作"
          width="190"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              size="small"
              link
              type="primary"
              @click="open(row)"
            >
              编辑
            </el-button>
            <el-button
              size="small"
              link
              @click="toggle(row)"
            >
              {{ row.status === 'enabled' ? '暂停' : '启用' }}
            </el-button>
            <el-button
              size="small"
              link
              type="danger"
              :disabled="row.status === 'enabled'"
              @click="remove(row)"
            >
              删除
            </el-button>
          </template>
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
  <el-dialog
    v-model="dialog"
    :title="editing ? '编辑伸缩策略' : '创建伸缩策略'"
    width="690px"
    top="5vh"
    :close-on-click-modal="false"
  >
    <div class="steps-wrap">
      <el-steps
        :active="step"
        finish-status="success"
        align-center
      >
        <el-step title="工作负载" />
        <el-step title="指标" />
        <el-step title="伸缩行为" />
        <el-step title="确认" />
      </el-steps>
    </div>
    <div v-if="step === 0">
      <p class="form-help">选择未被其他伸缩控制器管理的 Deployment。</p>
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="Namespace">
            <el-select
              v-model="form.namespace"
              style="width: 100%"
              :disabled="!!editing"
              @change="form.workloadName = ''"
            >
              <el-option
                v-for="n in [...new Set(workloads.map((w) => w.namespace))]"
                :key="n"
                :value="n"
                :label="n"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="Deployment">
            <el-select
              v-model="form.workloadName"
              style="width: 100%"
              :disabled="!!editing"
            >
              <el-option
                v-for="w in candidates"
                :key="w.name"
                :value="w.name"
                :label="`${w.name} · ${w.controllerSource}`"
                :disabled="
                  !editing &&
                  (!!w.policyId ||
                    (w.controllerSource !== 'unmanaged' && w.controllerSource !== 'manual'))
                "
              />
            </el-select>
          </el-form-item>
        </div>
      </el-form>
      <div
        v-if="selected"
        class="callout"
      >
        当前副本 {{ selected.desiredReplicas }}，Ready {{ selected.readyReplicas }}；控制来源
        {{ selected.controllerSource }}。
      </div>
    </div>
    <div v-else-if="step === 1">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="策略名称">
            <el-input
              v-model="form.name"
              placeholder="例如：checkout-api · CPU"
            />
          </el-form-item>
          <el-form-item label="决策指标">
            <el-select
              v-model="form.metric"
              style="width: 100%"
              @change="resetMetricTarget"
            >
              <el-option
                value="cpu_utilization"
                label="CPU 利用率"
              />
              <el-option
                value="rps"
                label="每秒请求数（RPS）"
              />
            </el-select>
          </el-form-item>
          <el-form-item
            :label="form.metric === 'rps' ? '每 Pod 目标 RPS' : '目标 CPU 利用率（0～1）'"
          >
            <el-input-number
              v-model="form.targetValue"
              :min="form.metric === 'rps' ? 1 : 0.01"
              :max="form.metric === 'rps' ? 100000 : 1"
              :step="form.metric === 'rps' ? 10 : 0.05"
            />
          </el-form-item>
          <el-form-item label="指标查询">
            <el-button
              :loading="querying"
              @click="testMetric"
            >
              测试查询
            </el-button>
          </el-form-item>
        </div>
      </el-form>
      <p
        v-if="queryResult"
        class="form-help"
      >
        当前值：{{ queryResult.current ?? '暂无数据' }} · 质量：{{ queryResult.quality }}
      </p>
      <p class="form-help">内存、延迟和错误率作为观测指标，暂不支持作为首版伸缩决策指标。</p>
    </div>
    <div v-else-if="step === 2">
      <el-form label-position="top">
        <div class="form-grid">
          <el-form-item label="最小副本">
            <el-input-number
              v-model="form.minReplicas"
              :min="1"
              :max="100"
            />
          </el-form-item>
          <el-form-item label="最大副本">
            <el-input-number
              v-model="form.maxReplicas"
              :min="1"
              :max="100"
            />
          </el-form-item>
          <el-form-item label="扩容连续周期">
            <el-input-number
              v-model="form.scaleUpConsecutivePeriods"
              :min="1"
              :max="20"
            />
          </el-form-item>
          <el-form-item label="缩容连续周期">
            <el-input-number
              v-model="form.scaleDownConsecutivePeriods"
              :min="1"
              :max="20"
            />
          </el-form-item>
          <el-form-item label="冷却期（秒）">
            <el-input-number
              v-model="form.cooldownSeconds"
              :min="0"
              :max="3600"
            />
          </el-form-item>
          <el-form-item label="缩容稳定窗口（秒）">
            <el-input-number
              v-model="form.stabilizationSeconds"
              :min="0"
              :max="3600"
            />
          </el-form-item>
          <el-form-item label="采样间隔（秒）">
            <el-input-number
              v-model="form.samplingIntervalSeconds"
              :min="1"
              :max="300"
            />
          </el-form-item>
        </div>
      </el-form>
    </div>
    <div
      v-else
      class="card info-list"
    >
      <div class="info-row">
        <span>目标</span>
        <strong>{{ form.namespace }} / {{ form.workloadName }}</strong>
      </div>
      <div class="info-row">
        <span>伸缩目标</span>
        <strong>
          {{
            form.metric === 'rps'
              ? `${form.targetValue} req/s / Pod`
              : `${Math.round(form.targetValue * 100)}% CPU`
          }}
        </strong>
      </div>
      <div class="info-row">
        <span>副本范围</span>
        <strong>{{ form.minReplicas }} – {{ form.maxReplicas }}</strong>
      </div>
      <div class="info-row">
        <span>安全控制</span>
        <strong>
          扩容 {{ form.scaleUpConsecutivePeriods }} 次 · 缩容
          {{ form.scaleDownConsecutivePeriods }} 次 · 冷却 {{ form.cooldownSeconds }} 秒
        </strong>
      </div>
      <p class="form-help">保存后可在策略列表中启用。启用前后端会再次校验控制权。</p>
    </div>
    <template #footer>
      <el-button @click="dialog = false">取消</el-button>
      <el-button
        v-if="step > 0"
        @click="step--"
      >
        上一步
      </el-button>
      <el-button
        v-if="step < 3"
        type="primary"
        @click="next"
      >
        下一步
      </el-button>
      <el-button
        v-else
        type="primary"
        :loading="saving"
        @click="save"
      >
        保存策略
      </el-button>
    </template>
  </el-dialog>
</template>
