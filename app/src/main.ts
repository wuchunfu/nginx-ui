import { autoAnimatePlugin } from '@formkit/auto-animate/vue'
import { createPinia } from 'pinia'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import { createApp } from 'vue'
import VueDOMPurifyHTML from 'vue-dompurify-html'
import { useSettingsStore } from '@/pinia'
import App from './App.vue'
import gettext from './gettext'
import router from './routes'
import 'virtual:uno.css'

const pinia = createPinia()

const app = createApp(App)

pinia.use(piniaPluginPersistedstate)
app.use(pinia)
app.use(gettext)
app.use(VueDOMPurifyHTML, {
  hooks: {
    uponSanitizeElement: (node, data) => {
      if (node.tagName && node.tagName.toLowerCase() === 'think') {
        data.allowedTags.think = true
      }
    },
  },
})

// after pinia created
const settings = useSettingsStore()

gettext.current = settings.language || 'en'

app.use(router).use(autoAnimatePlugin).mount('#app')

export default app
