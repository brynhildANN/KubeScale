// 前端启动入口：创建 Vue 应用，注册 Pinia、路由、Element Plus 和全局样式。
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import './styles.css'

createApp(App).use(createPinia()).use(router).use(ElementPlus).mount('#app')
