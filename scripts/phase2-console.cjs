// 阶段二浏览器验收：连接真实模式控制台，只管理本脚本创建的策略。
// 需要外部安装 Playwright；通过 NODE_PATH 指向其 node_modules，不修改项目依赖。
const { chromium } = require('playwright')
const assert = require('node:assert/strict')
const fs = require('node:fs/promises')

const base = process.env.CONSOLE_URL || 'http://127.0.0.1:15173'
const namespace = process.env.TEST_NAMESPACE || 'kubescale-phase2-test'
const workload = process.env.TEST_WORKLOAD || 'demo-custom'
const output = process.env.RESULT_DIR || 'tests/results/phase2'
const name = `phase2-console-${Date.now()}`

;(async () => {
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {}),
  })
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } })
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  let id
  async function request(path, options) {
    const response = await page.request.fetch(`${base}/api/v1${path}`, options)
    assert(response.ok(), `${response.status()} ${await response.text()}`)
    return response.status() === 204 ? null : response.json()
  }
  async function row() {
    return page.locator('.el-table__body tr').filter({ hasText: name })
  }
  try {
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
    const list = await request('/policies')
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
    await page.getByRole('button', { name: '查看决策详情' }).first().click()
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
  } finally {
    if (id) {
      const p = await request(`/policies/${id}`)
      await request(`/policies/${id}/status`, { method: 'PATCH', data: { status: 'paused', version: p.version } })
      await request(`/policies/${id}`, { method: 'DELETE' })
    }
    await browser.close()
  }
})().catch((error) => { console.error(error); process.exitCode = 1 })
