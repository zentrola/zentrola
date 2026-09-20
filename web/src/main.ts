import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import { i18n } from './i18n'
import './style.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('./pages/Home.vue') },
    { path: '/members', name: 'members', component: () => import('./pages/Members.vue') },
    { path: '/groups', name: 'groups', component: () => import('./pages/Groups.vue') },
    { path: '/models', name: 'models', component: () => import('./pages/Models.vue') },
    { path: '/providers', name: 'providers', component: () => import('./pages/Providers.vue') },
    { path: '/usage', name: 'usage', component: () => import('./pages/Usage.vue') },
    {
      path: '/operations',
      name: 'operations',
      component: () => import('./pages/Operations.vue'),
    },
    { path: '/resources', redirect: '/providers' },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
createApp(App).use(i18n).use(router).mount('#app')
