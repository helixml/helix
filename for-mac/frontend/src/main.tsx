import { createRoot } from 'react-dom/client';
import { App } from './App';

const container = document.getElementById('app');
if (!container) throw new Error('Root element #app not found');
createRoot(container).render(<App />);
