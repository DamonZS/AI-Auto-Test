import { createApp } from 'vue'
import App from './App.vue'
import { installSelectPicker } from './selectPicker'
import './style.css'

installSelectPicker()
createApp(App).mount('#app')
