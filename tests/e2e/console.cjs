// 浏览器验收：策略生命周期、版本冲突、决策详情与真实指标图表。
// 需要外部安装 Playwright；通过 NODE_PATH 指向其 node_modules，不修改项目依赖。
const { chromium } = require('playwright')
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const base = process.env.CONSOLE_URL || 'http://127.0.0.1:5173'
const namespace = process.env.TEST_NAMESPACE || 'kubescale-test-e2e'
const workload = process.env.TEST_WORKLOAD || 'demo-custom'
const output = process.env.RESULT_DIR || path.resolve(__dirname, '../results/console')
const name = `console-acceptance-${Date.now()}`

assert(namespace.startsWith('kubescale-test-'), 'use a disposable kubescale-test-* namespace')

;(async () => {
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {}),
  })
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  let id
  let originalReplicas
  async function request(path, options) {
    const response = await page.request.fetch(`${base}/api/v1${path}`, options)
    assert(response.ok(), `${response.status()} ${await response.text()}`)
    return response.status() === 204 ? null : response.json()
  }
  async function row() {
    return page.locator('.el-table__body tr').filter({ hasText: name })
  }
  try {
    await fs.mkdir(output, { recursive: true })
    assert.equal((await request('/policies')).total, 0, 'test backend must have no existing policies')
    originalReplicas = (await request(`/workloads/${namespace}/${workload}`)).desiredReplicas
    await page.goto(`${base}/policies`)
    await page.getByRole('heading', { name: '伸缩策略', exact: true }).waitFor()
    await page.getByText('每 30 秒自动刷新 · 支持手动刷新').waitFor()
    await page.getByRole('button', { name: '创建策略' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.locator('.el-form-item').filter({ hasText: 'Namespace' }).locator('.el-select').click()
    await page.getByRole('option', { name: namespace, exact: true }).click()
    await dialog.locator('.el-form-item').filter({ hasText: 'Deployment' }).locator('.el-select').click()
    await page.getByRole('option', { name: new RegExp(`^${workload} ·`) }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.locator('.el-form-item').filter({ hasText: '策略名称' }).locator('input').fill(name)
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '保存策略' }).click()
    await dialog.waitFor({ state: 'hidden' })
    const list = await request(`/policies?namespace=${encodeURIComponent(namespace)}&pageSize=100`)
    const created = list.items.find((p) => p.name === name)
    assert(created, 'created policy missing')
    id = created.id
    await (await row()).getByRole('button', { name: '启用', exact: true }).click()
    await (await row()).getByRole('button', { name: '暂停', exact: true }).waitFor()
    await (await row()).getByRole('button', { name: '编辑', exact: true }).click()
    await dialog.locator('.el-form-item').filter({ hasText: 'Namespace' }).waitFor()
    const current = await request(`/policies/${id}`)
    await request(`/policies/${id}`, { method: 'PUT', data: { ...current, targetValue: .6 } })
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '保存策略' }).click()
    await page.getByText('策略已被修改，请刷新列表并重新打开编辑后再提交').waitFor()
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await (await row()).getByRole('button', { name: '编辑', exact: true }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    const input = dialog.locator('.el-form-item').filter({ hasText: '目标 CPU 利用率' }).locator('input')
    await input.fill('0.65')
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '下一步' }).click()
    await dialog.getByRole('button', { name: '保存策略' }).click()
    await dialog.waitFor({ state: 'hidden' })
    assert.equal((await request(`/policies/${id}`)).targetValue, .65)
    await page.goto(`${base}/workloads/${namespace}/${workload}`)
    await page.getByRole('tab', { name: '自动伸缩', exact: true }).click()
    await page.getByRole('button', { name: '查看决策详情' }).first().click({ timeout: 120000 })
    await page.getByText('决策时的策略配置').waitFor()
    await page.getByText('指标采样时间', { exact: true }).waitFor()
    await fs.mkdir(output, { recursive: true })
    await page.screenshot({ path: `${output}/decision.png`, fullPage: true })
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: '暂停策略', exact: true }).click()
    await page.getByRole('button', { name: '恢复策略', exact: true }).click()
    await page.getByRole('button', { name: '暂停策略', exact: true }).waitFor()
    await page.getByRole('tab', { name: '指标', exact: true }).click()
    await page.locator('canvas:visible').first().waitFor()
    await page.screenshot({ path: `${output}/metrics.png`, fullPage: true })
    await page.getByRole('button', { name: '暂停策略', exact: true }).click()
    await page.goto(`${base}/policies`)
    await (await row()).getByRole('button', { name: '删除', exact: true }).click()
    await page.locator('.el-message-box').getByRole('button', { name: '删除', exact: true }).click()
    await page.getByText('已删除', { exact: true }).waitFor()
    assert(!(await request('/policies')).items.some((p) => p.id === id))
    id = null
    assert.deepEqual(errors, [])
    await fs.writeFile(`${output}/console.json`, JSON.stringify({ passed: true, checks: ['create', 'enable', 'version conflict', 'edit', 'decision snapshot', 'charts', 'pause', 'resume', 'delete', 'no browser errors'] }, null, 2))
    console.log('PASS: real console lifecycle, conflict feedback, decision detail and charts')
  } catch (error) {
    await fs.mkdir(output, { recursive: true })
    await fs.writeFile(`${output}/console.json`, JSON.stringify({ passed: false, error: error.message }, null, 2))
    throw error
  } finally {
    try {
      // 保存请求已成功但页面后续步骤失败时，也按唯一名称找回本次策略。
      if (!id) {
        const list = await request(`/policies?namespace=${encodeURIComponent(namespace)}&pageSize=100`)
        id = list.items.find((p) => p.name === name)?.id
      }
      if (id) {
        const p = await request(`/policies/${id}`)
        await request(`/policies/${id}/status`, { method: 'PATCH', data: { status: 'paused', version: p.version } })
        await request(`/policies/${id}`, { method: 'DELETE' })
      }
      if (originalReplicas !== undefined) {
        await request(`/workloads/${namespace}/${workload}/scale`, {
          method: 'POST', data: { replicas: originalReplicas, reason: 'console acceptance restore' },
        })
      }
    } catch (error) {
      await fs.mkdir(output, { recursive: true })
      await fs.writeFile(`${output}/console.json`, JSON.stringify({ passed: false, cleanupError: error.message }, null, 2))
      throw error
    } finally {
      await browser.close()
    }
  }
})().catch((error) => { console.error(error); process.exitCode = 1 })
