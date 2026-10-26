// 页面路由：配置概览、工作负载、策略、操作、告警与设置，按需加载页面。
import { createRouter, createWebHistory } from 'vue-router'

export default createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'overview',
      component: () => import('@/views/OverviewView.vue'),
      meta: { title: '概览' },
    },
    {
      path: '/workloads',
      name: 'workloads',
      component: () => import('@/views/WorkloadsView.vue'),
      meta: { title: '工作负载' },
    },
    {
      path: '/workloads/:namespace/:name',
      name: 'workload',
      component: () => import('@/views/WorkloadDetailView.vue'),
      meta: { title: '工作负载详情' },
    },
    {
      path: '/policies',
      name: 'policies',
      component: () => import('@/views/PoliciesView.vue'),
      meta: { title: '伸缩策略' },
    },
    {
      path: '/operations',
      name: 'operations',
      component: () => import('@/views/OperationsView.vue'),
      meta: { title: '操作记录' },
    },
    {
      path: '/alerts',
      name: 'alerts',
      component: () => import('@/views/AlertsView.vue'),
      meta: { title: '告警中心' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      meta: { title: '系统设置' },
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})
