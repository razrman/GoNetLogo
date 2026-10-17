import { createRoot } from 'react-dom/client'
import { Provider } from 'urql'
import { client } from './api'
import { App } from './App'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <Provider value={client}>
    <App />
  </Provider>,
)
