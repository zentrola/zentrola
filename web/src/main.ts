import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import { i18n } from './i18n'
import Members from './pages/Members.vue'
import Groups from './pages/Groups.vue'
import Models from './pages/Models.vue'
import Providers from './pages/Providers.vue'
import Usage from './pages/Usage.vue'
import Operations from './pages/Operations.vue'
import './style.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/members' },
    ...Object.entries({
      members: Members,
      groups: Groups,
      models: Models,
      providers: Providers,
      usage: Usage,
      operations: Operations,
    }).map(([name, component]) => ({ path: `/${name}`, name, component })),
    { path: '/resources', redirect: '/providers' },
    { path: '/:pathMatch(.*)*', redirect: '/members' },
  ],
})
createApp(App).use(i18n).use(router).mount('#app')
