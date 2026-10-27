import http from 'k6/http'
import { check, sleep } from 'k6'

const target = __ENV.TARGET
if (!target || !/^https?:\/\//.test(target)) throw new Error('Set TARGET=http(s)://<service-endpoint>')
const work = Number(__ENV.WORK || 120)
if (!Number.isInteger(work) || work < 0) throw new Error('WORK must be a non-negative integer')
export const options = {
  thresholds: { http_req_failed: ['rate<0.01'], checks: ['rate>0.99'] },
  stages: [
    { duration: '1m', target: 10 },
    { duration: '3m', target: 40 },
    { duration: '2m', target: 0 },
  ],
}
export default function () {
  const response = http.get(`${target}${target.includes('?') ? '&' : '?'}work=${work}`)
  check(response, { 'HTTP 200': (r) => r.status === 200 })
  sleep(0.2)
}
