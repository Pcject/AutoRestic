import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: () => import('../views/Dashboard.vue') },
    { path: '/repos', name: 'repos', component: () => import('../views/Repos.vue') },
    { path: '/repos/:id', name: 'repo-detail', component: () => import('../views/RepoDetail.vue') },
    { path: '/tasks', name: 'tasks', component: () => import('../views/Tasks.vue') },
    { path: '/snapshots', name: 'snapshots', component: () => import('../views/Snapshots.vue') },
    { path: '/logs', name: 'logs', component: () => import('../views/Logs.vue') },
    { path: '/settings', name: 'settings', component: () => import('../views/Settings.vue') }
  ]
})

export default router