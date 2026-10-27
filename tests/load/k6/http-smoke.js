import http from 'k6/http'
import { check, sleep } from 'k6'

// 读取并校验目标地址
const target = __ENV.TARGET
if (!target || !/^https?:\/\//.test(target)) {
  throw new Error('Set TARGET=http(s)://<service-endpoint>')
}

// 读取并校验请求参数
const work = Number(__ENV.WORK || 120)
if (!Number.isInteger(work) || work < 0) {
  throw new Error('WORK must be a non-negative integer')
}

// 压测配置
export const options = {
  // 压力测试的验收标准
  thresholds: {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
  stages: [
    { duration: '1m', target: 10 },
    { duration: '3m', target: 40 },
    { duration: '2m', target: 0 },
  ],
}

// 每个虚拟用户反复执行的操作
export default function () {
  const response = http.get(`${target}${target.includes('?') ? '&' : '?'}work=${work}`)

  check(response, {
    'HTTP 200': (r) => r.status === 200,
  })

  sleep(0.2)
}
