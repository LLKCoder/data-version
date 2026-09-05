import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: '/dashboards/ops-overview',
    },
    {
      path: '/dashboards/:uid',
      component: () => import('@/pages/DashboardView.vue'),
    },
    {
      path: '/dashboards/new',
      component: () => import('@/pages/DashboardView.vue'),
    },
    {
      path: '/datasources',
      component: () => import('@/pages/DataSourceView.vue'),
    },
  ],
})

export default router
