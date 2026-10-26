import http from 'k6/http'
import { check, sleep } from 'k6'

const target = __ENV.TARGET || 'http://127.0.0.1:8081'
export const options = {
  stages: [
    { duration: '1m', target: 10 },
    { duration: '3m', target: 40 },
    { duration: '2m', target: 0 },
  ],
}
export default function () {
  const response = http.get(`${target}/?work=120`)
  check(response, { 'HTTP 200': (r) => r.status === 200 })
  sleep(0.2)
}
