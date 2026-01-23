package services

import (
	"context"
	"fmt"
)

// SampleProjectCodeService manages hardcoded starter code for sample projects
type SampleProjectCodeService struct {
	sampleProjects map[string]*SampleProjectCode
}

// SampleProjectCode contains the starter code and structure for a sample project
type SampleProjectCode struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	GitHubRepo    string            `json:"github_repo"`
	Technologies  []string          `json:"technologies"`
	Files         map[string]string `json:"files"` // filepath -> content
	GitIgnore     string            `json:"gitignore"`
	ReadmeURL     string            `json:"readme_url"`
	Language      string            `json:"language"`
	StartupScript string            `json:"startup_script"` // Custom startup script for this project
}

// NewSampleProjectCodeService creates a new service with hardcoded project codes
func NewSampleProjectCodeService() *SampleProjectCodeService {
	service := &SampleProjectCodeService{
		sampleProjects: make(map[string]*SampleProjectCode),
	}
	service.loadSampleProjects()
	return service
}

// GetProjectCode returns the starter code for a given project ID
func (s *SampleProjectCodeService) GetProjectCode(ctx context.Context, projectID string) (*SampleProjectCode, error) {
	project, exists := s.sampleProjects[projectID]
	if !exists {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	return project, nil
}

// ListAvailableProjects returns all available sample projects
func (s *SampleProjectCodeService) ListAvailableProjects(ctx context.Context) []*SampleProjectCode {
	projects := make([]*SampleProjectCode, 0, len(s.sampleProjects))
	for _, project := range s.sampleProjects {
		projects = append(projects, project)
	}
	return projects
}

// loadSampleProjects initializes the hardcoded sample project starter codes
func (s *SampleProjectCodeService) loadSampleProjects() {
	// 1. Modern Todo App - has bug where delete doesn't persist to localStorage
	s.sampleProjects["modern-todo-app"] = &SampleProjectCode{
		ID:           "modern-todo-app",
		Name:         "Modern Todo App",
		Description:  "Full-stack todo application with React - perfect for learning modern web patterns",
		GitHubRepo:   "helixml/sample-todo-app",
		Technologies: []string{"React", "TypeScript", "Vite"},
		Language:     "javascript",
		ReadmeURL:    "https://github.com/helixml/sample-todo-app/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Script runs in primary repository directory (wrapper already cd'd here)
echo "📂 Working in: $(pwd)"

# Verify we're in a Node.js project
if [ ! -f "package.json" ]; then
    echo "❌ Error: package.json not found in current directory"
    exit 1
fi

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

echo "🚀 Installing dependencies..."
npm install

echo "🚀 Starting dev server in background..."
nohup npm run dev > /tmp/dev-server.log 2>&1 &
DEV_PID=$!

echo "Dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/dev-server.log"

# Wait for server to be ready
sleep 5

# Open browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:3000 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Todo app running at http://localhost:3000"
`,
		GitIgnore: `node_modules/
dist/
.env`,
		Files: map[string]string{
			"README.md": `# Modern Todo App

A simple todo app with a critical bug to fix!

## Bug
⚠️ Deleted todos reappear after page refresh - the delete doesn't persist to localStorage

## Getting Started
` + "```bash" + `
npm install
npm run dev
` + "```",
			"package.json": `{
  "name": "modern-todo-app",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^4.4.0"
  }
}`,
			"index.html": `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Todo App</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>`,
			"src/main.tsx": `import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)`,
			"src/App.tsx": `import { useState, useEffect } from 'react';

interface Todo {
  id: number;
  text: string;
  completed: boolean;
}

function App() {
  const [todos, setTodos] = useState<Todo[]>([]);
  const [input, setInput] = useState('');

  useEffect(() => {
    const saved = localStorage.getItem('todos');
    if (saved) setTodos(JSON.parse(saved));
  }, []);

  const addTodo = () => {
    if (!input.trim()) return;
    const newTodos = [...todos, { id: Date.now(), text: input, completed: false }];
    setTodos(newTodos);
    localStorage.setItem('todos', JSON.stringify(newTodos));
    setInput('');
  };

  const toggleTodo = (id: number) => {
    const newTodos = todos.map(t => t.id === id ? {...t, completed: !t.completed} : t);
    setTodos(newTodos);
    localStorage.setItem('todos', JSON.stringify(newTodos));
  };

  const deleteTodo = (id: number) => {
    const newTodos = todos.filter(t => t.id !== id);
    setTodos(newTodos);
    // BUG: Missing localStorage save - todos come back after refresh!
  };

  return (
    <div style={{maxWidth: '600px', margin: '0 auto', padding: '20px'}}>
      <h1>Todo App</h1>
      <div style={{display: 'flex', gap: '10px', marginBottom: '20px'}}>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyPress={(e) => e.key === 'Enter' && addTodo()}
          placeholder="Add a todo..."
          style={{flex: 1, padding: '8px'}}
        />
        <button onClick={addTodo}>Add</button>
      </div>
      <ul style={{listStyle: 'none', padding: 0}}>
        {todos.map(todo => (
          <li key={todo.id} style={{padding: '10px', borderBottom: '1px solid #ccc', display: 'flex', alignItems: 'center', gap: '10px'}}>
            <input
              type="checkbox"
              checked={todo.completed}
              onChange={() => toggleTodo(todo.id)}
            />
            <span style={{flex: 1, textDecoration: todo.completed ? 'line-through' : 'none'}}>
              {todo.text}
            </span>
            <button onClick={() => deleteTodo(todo.id)}>Delete</button>
          </li>
        ))}
      </ul>
    </div>
  );
}

export default App;`,
			"src/index.css": `body {
  font-family: system-ui, sans-serif;
  margin: 0;
  padding: 0;
}

button {
  padding: 8px 16px;
  cursor: pointer;
}

input[type="text"] {
  border: 1px solid #ccc;
  border-radius: 4px;
}`,
			"vite.config.ts": `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 3000, host: '0.0.0.0' }
})`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["ES2020", "DOM"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true
  }
}`,
		},
	}

	// 2. E-commerce API - has inventory race condition bug
	s.sampleProjects["ecommerce-api"] = &SampleProjectCode{
		ID:           "ecommerce-api",
		Name:         "E-commerce REST API",
		Description:  "API for an e-commerce platform with product management and orders",
		GitHubRepo:   "helixml/sample-ecommerce-api",
		Technologies: []string{"Node.js", "Express", "TypeScript"},
		Language:     "javascript",
		ReadmeURL:    "https://github.com/helixml/sample-ecommerce-api/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Script runs in primary repository directory (wrapper already cd'd here)
echo "📂 Working in: $(pwd)"

# Verify we're in a Node.js project
if [ ! -f "package.json" ]; then
    echo "❌ Error: package.json not found in current directory"
    exit 1
fi

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

echo "🚀 Installing dependencies..."
npm install

echo "🚀 Starting API server in background..."
nohup npm run dev > /tmp/api-server.log 2>&1 &
DEV_PID=$!

echo "API server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/api-server.log"

# Wait for server to be ready
sleep 3

echo "✅ Startup complete - E-commerce API running at http://localhost:3000"
echo "📝 Test with: curl http://localhost:3000/api/products"
`,
		GitIgnore: `node_modules/
dist/
.env`,
		Files: map[string]string{
			"README.md": `# E-commerce API

Simple e-commerce REST API.

## Critical Bug
⚠️ Inventory can go negative when multiple users order simultaneously (race condition)

## API Endpoints
- GET /api/products - List products
- POST /api/orders - Create order

## Start
` + "```bash" + `
npm install && npm run dev
` + "```",
			"package.json": `{
  "name": "ecommerce-api",
  "version": "1.0.0",
  "scripts": {
    "dev": "ts-node src/index.ts"
  },
  "dependencies": {
    "express": "^4.18.0",
    "cors": "^2.8.5"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "@types/express": "^4.17.0",
    "@types/cors": "^2.8.0",
    "typescript": "^5.0.0",
    "ts-node": "^10.9.0"
  }
}`,
			"src/index.ts": `import express from 'express';
import cors from 'cors';

const app = express();
app.use(cors());
app.use(express.json());

// In-memory inventory (simulates database)
const inventory: Record<string, { name: string; price: number; stock: number }> = {
  'prod-1': { name: 'Widget', price: 29.99, stock: 10 },
  'prod-2': { name: 'Gadget', price: 49.99, stock: 5 },
};

app.get('/api/products', (req, res) => {
  res.json(Object.entries(inventory).map(([id, data]) => ({ id, ...data })));
});

app.post('/api/orders', async (req, res) => {
  const { productId, quantity } = req.body;

  // BUG: Race condition! No locking mechanism
  const product = inventory[productId];
  if (!product) return res.status(404).json({ error: 'Product not found' });

  // Simulate async delay that makes race condition worse
  await new Promise(resolve => setTimeout(resolve, 100));

  // BUG: Check happens BEFORE the delay, inventory gets decremented AFTER
  if (product.stock < quantity) {
    return res.status(400).json({ error: 'Insufficient inventory' });
  }

  // BUG: Another request can pass the check above before this executes!
  product.stock -= quantity;

  res.json({ message: 'Order placed', remaining: product.stock });
});

app.listen(3000, () => console.log('API on http://localhost:3000'));`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "esModuleInterop": true
  }
}`,
		},
	}

	// 3. Weather App - React Native (minimal, missing offline mode)
	s.sampleProjects["weather-app"] = &SampleProjectCode{
		ID:           "weather-app",
		Name:         "Weather App - React Native",
		Description:  "Cross-platform weather app with location services",
		GitHubRepo:   "helixml/sample-weather-app",
		Technologies: []string{"React Native", "TypeScript", "Expo"},
		Language:     "javascript",
		ReadmeURL:    "https://github.com/helixml/sample-weather-app/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Script runs in primary repository directory (wrapper already cd'd here)
echo "📂 Working in: $(pwd)"

# Verify we're in a Node.js project
if [ ! -f "package.json" ]; then
    echo "❌ Error: package.json not found in current directory"
    exit 1
fi

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

echo "🚀 Installing dependencies..."
npm install

echo "🚀 Starting Expo dev server in background..."
nohup npx expo start --web > /tmp/expo-server.log 2>&1 &
DEV_PID=$!

echo "Expo dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/expo-server.log"

# Wait for server to be ready
sleep 5

# Open browser for web preview
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:19006 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Expo running at http://localhost:19006"
echo "📱 Scan QR code in terminal to run on mobile device"
`,
		GitIgnore: `node_modules/
.expo/
dist/`,
		Files: map[string]string{
			"README.md": `# Weather App

React Native weather app.

## Missing Features
- ⚠️ No offline mode - app crashes without internet
- No weather animations

## Start
` + "```bash" + `
npm install && npx expo start
` + "```",
			"package.json": `{
  "name": "weather-app",
  "version": "1.0.0",
  "main": "node_modules/expo/AppEntry.js",
  "scripts": {
    "start": "expo start"
  },
  "dependencies": {
    "expo": "~49.0.0",
    "react": "18.2.0",
    "react-native": "0.72.0"
  },
  "devDependencies": {
    "@types/react": "~18.2.0",
    "typescript": "^5.0.0"
  }
}`,
			"App.tsx": `import { useState, useEffect } from 'react';
import { View, Text, StyleSheet } from 'react-native';

export default function App() {
  const [weather, setWeather] = useState<any>(null);

  useEffect(() => {
    // BUG: No offline handling - this will crash without internet!
    fetch('https://api.open-meteo.com/v1/forecast?latitude=52.52&longitude=13.41&current_weather=true')
      .then(res => res.json())
      .then(data => setWeather(data.current_weather));
  }, []);

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Weather</Text>
      {weather ? (
        <Text>Temperature: {weather.temperature}°C</Text>
      ) : (
        <Text>Loading...</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, justifyContent: 'center', alignItems: 'center' },
  title: { fontSize: 24, fontWeight: 'bold', marginBottom: 20 },
});`,
			"app.json": `{
  "expo": {
    "name": "weather-app",
    "slug": "weather-app",
    "version": "1.0.0",
    "platforms": ["ios", "android"],
    "sdkVersion": "49.0.0"
  }
}`,
			"tsconfig.json": `{
  "compilerOptions": {
    "strict": true
  },
  "extends": "expo/tsconfig.base"
}`,
		},
	}

	// 4. Blog CMS - Next.js (missing image uploads)
	s.sampleProjects["blog-cms"] = &SampleProjectCode{
		ID:           "blog-cms",
		Name:         "Simple Blog CMS",
		Description:  "Content management system for bloggers with markdown support",
		GitHubRepo:   "helixml/sample-blog-cms",
		Technologies: []string{"Next.js", "TypeScript", "TailwindCSS"},
		Language:     "javascript",
		ReadmeURL:    "https://github.com/helixml/sample-blog-cms/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Script runs in primary repository directory (wrapper already cd'd here)
echo "📂 Working in: $(pwd)"

# Verify we're in a Node.js project
if [ ! -f "package.json" ]; then
    echo "❌ Error: package.json not found in current directory"
    exit 1
fi

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

echo "🚀 Installing dependencies..."
npm install

echo "🚀 Starting Next.js dev server in background..."
nohup npm run dev > /tmp/nextjs-server.log 2>&1 &
DEV_PID=$!

echo "Next.js dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/nextjs-server.log"

# Wait for server to be ready
sleep 5

# Open browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:3000 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Blog CMS running at http://localhost:3000"
`,
		GitIgnore: `node_modules/
.next/
out/`,
		Files: map[string]string{
			"README.md": `# Blog CMS

Simple blog with markdown posts.

## Missing
- ⚠️ No image upload functionality
- No comments
- No SEO meta tags

## Start
` + "```bash" + `
npm install && npm run dev
` + "```",
			"package.json": `{
  "name": "blog-cms",
  "version": "0.1.0",
  "scripts": {
    "dev": "next dev",
    "build": "next build"
  },
  "dependencies": {
    "next": "14.0.0",
    "react": "^18.2.0",
    "react-dom": "^18.2.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "@types/react": "^18.2.0",
    "typescript": "^5.0.0"
  }
}`,
			"app/page.tsx": `export default function Home() {
  const posts = [
    { id: 1, title: 'First Post', content: 'Hello world' },
  ];

  return (
    <main style={{maxWidth: '800px', margin: '0 auto', padding: '20px'}}>
      <h1>Blog</h1>
      {posts.map(post => (
        <article key={post.id}>
          <h2>{post.title}</h2>
          <p>{post.content}</p>
        </article>
      ))}
    </main>
  );
}`,
			"app/layout.tsx": `export const metadata = {
  title: 'Blog CMS',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2017",
    "lib": ["dom", "dom.iterable", "esnext"],
    "module": "esnext",
    "moduleResolution": "bundler",
    "jsx": "preserve",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  }
}`,
			"next.config.js": `/** @type {import('next').NextConfig} */
const nextConfig = {};
module.exports = nextConfig;`,
		},
	}

	// 5. React Dashboard - Material-UI (missing real-time updates)
	s.sampleProjects["react-dashboard"] = &SampleProjectCode{
		ID:           "react-dashboard",
		Name:         "React Admin Dashboard",
		Description:  "Modern admin dashboard with Material-UI components",
		GitHubRepo:   "helixml/sample-react-dashboard",
		Technologies: []string{"React", "TypeScript", "Material-UI"},
		Language:     "javascript",
		ReadmeURL:    "https://github.com/helixml/sample-react-dashboard/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Script runs in primary repository directory (wrapper already cd'd here)
echo "📂 Working in: $(pwd)"

# Verify we're in a Node.js project
if [ ! -f "package.json" ]; then
    echo "❌ Error: package.json not found in current directory"
    exit 1
fi

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

echo "🚀 Installing dependencies..."
npm install

echo "🚀 Starting dev server in background..."
nohup npm run dev > /tmp/dev-server.log 2>&1 &
DEV_PID=$!

echo "Dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/dev-server.log"

# Wait for server to be ready
sleep 5

# Open browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:3000 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Dashboard running at http://localhost:3000"
`,
		GitIgnore: `node_modules/
dist/`,
		Files: map[string]string{
			"README.md": `# React Admin Dashboard

Dashboard with static data.

## Missing
- ⚠️ No real-time updates (data doesn't refresh)
- No role-based access control
- No export functionality

## Start
` + "```bash" + `
npm install && npm run dev
` + "```",
			"package.json": `{
  "name": "react-dashboard",
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "@mui/material": "^5.14.0",
    "@emotion/react": "^11.11.0",
    "@emotion/styled": "^11.11.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^4.4.0"
  }
}`,
			"index.html": `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Dashboard</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>`,
			"src/main.tsx": `import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)`,
			"src/App.tsx": `import { Container, Typography, Paper, Grid } from '@mui/material';

function App() {
  // Static data - BUG: Should update in real-time via WebSocket
  const metrics = {
    users: 1234,
    sales: 5678,
    revenue: 91011,
  };

  return (
    <Container maxWidth="lg" sx={{ py: 4 }}>
      <Typography variant="h4" gutterBottom>Admin Dashboard</Typography>
      <Grid container spacing={3}>
        <Grid item xs={12} md={4}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6">Users</Typography>
            <Typography variant="h3">{metrics.users}</Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} md={4}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6">Sales</Typography>
            <Typography variant="h3">{metrics.sales}</Typography>
          </Paper>
        </Grid>
        <Grid item xs={12} md={4}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6">Revenue</Typography>
            <Typography variant="h3">${metrics.revenue}</Typography>
          </Paper>
        </Grid>
      </Grid>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
        ⚠️ Data is static - implement WebSocket for real-time updates
      </Typography>
    </Container>
  );
}

export default App;`,
			"vite.config.ts": `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 3000 }
})`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "jsx": "react-jsx",
    "strict": true
  }
}`,
		},
	}

	// 6. LinkedIn Outreach - Campaign tracker (markdown templates)
	s.sampleProjects["linkedin-outreach"] = &SampleProjectCode{
		ID:           "linkedin-outreach",
		Name:         "LinkedIn Outreach Campaign",
		Description:  "Multi-session campaign to reach out to 100 prospects",
		GitHubRepo:   "helixml/sample-linkedin-outreach",
		Technologies: []string{"Markdown", "CSV"},
		Language:     "markdown",
		ReadmeURL:    "https://github.com/helixml/sample-linkedin-outreach/blob/main/README.md",
		StartupScript: `#!/bin/bash
set -euo pipefail

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

# Fix ownership - directories are cloned as root, need to be owned by retro
sudo chown -R retro:retro .

echo "🚀 LinkedIn Outreach Campaign workspace ready"
echo ""
echo "📋 Campaign files:"
echo "  - prospects.csv: Prospect tracking"
echo "  - message-templates.md: Outreach templates"
echo "  - metrics.md: Campaign metrics"
echo ""
echo "💡 Start by filling out prospects.csv with your target list"
echo "✅ Workspace ready - no dev server needed"
`,
		GitIgnore: `*.csv.tmp
.env`,
		Files: map[string]string{
			"README.md": `# LinkedIn Outreach Campaign

Campaign to reach 100 AI/ML prospects.

## Tasks
1. Build prospect list
2. Write personalized messages
3. Track metrics
4. Create follow-up sequence

## Files
- prospects.csv - Prospect list (empty - to be filled)
- message-templates.md - Outreach templates
- metrics.md - Campaign tracking`,
			"prospects.csv": `name,title,company,linkedin_url,status,notes
"","","","",pending,""
`,
			"message-templates.md": `# Message Templates

## Initial Connection Request
Hi {name}, I noticed your work on {recent_activity}. Would love to connect!

## Follow-up 1
Following up on my connection request...

## Follow-up 2
Wanted to share {value_add}...`,
			"metrics.md": `# Campaign Metrics

## Overview
- Target: 100 prospects
- Messages Sent: 0
- Responses: 0
- Meetings: 0

## Tracking
Update this file as campaign progresses.`,
		},
	}

	// Jupyter Financial Analysis - Main Project Entry
	// This provides the startup script for the overall project
	s.sampleProjects["jupyter-financial-analysis"] = &SampleProjectCode{
		ID:           "jupyter-financial-analysis",
		Name:         "Jupyter Financial Analysis",
		Description:  "Financial data analysis using Jupyter notebooks with S&P 500 data",
		Technologies: []string{"Python", "Jupyter", "Pandas", "Finance"},
		Language:     "python",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Add user's local bin to PATH for pip-installed executables (for agent shells)
if ! grep -q 'export PATH="$HOME/.local/bin:$PATH"' ~/.bashrc; then
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
    echo "✅ Added ~/.local/bin to PATH in ~/.bashrc"
fi

# Also set for current script
export PATH="$HOME/.local/bin:$PATH"

# Install Python 3 and pip if not present
if ! command -v python3 &> /dev/null; then
    echo "📦 Installing Python 3..."
    sudo apt-get update
    sudo apt-get install -y python3 python3-pip python3-venv
fi

# Ensure pip3 is available
if ! command -v pip3 &> /dev/null; then
    echo "📦 Installing pip3..."
    sudo apt-get update
    sudo apt-get install -y python3-pip
fi

# Install pyforest library from sibling repository
# Use --break-system-packages for container development environment (Ubuntu 25.04 PEP 668)
if [ -d "../pyforest" ]; then
    echo "📦 Installing pyforest library..."
    pip3 install --break-system-packages -e ../pyforest
else
    echo "⚠️ Warning: pyforest repository not found at ../pyforest"
    echo "Install it manually or attach the pyforest repository to this project"
fi

# Install requirements
if [ -f "requirements.txt" ]; then
    echo "📦 Installing Python dependencies..."
    pip3 install --break-system-packages -r requirements.txt
fi

echo "🚀 Starting Jupyter Lab in background..."
echo "Access Jupyter at http://localhost:8888"
echo ""
echo "Agent Commands:"
echo "  - Run notebook: jupyter nbconvert --execute --to notebook --inplace portfolio_analysis.ipynb"
echo "  - Generate HTML: jupyter nbconvert --execute --to html portfolio_analysis.ipynb"
echo "  - View HTML: open portfolio_analysis.html (or xdg-open on Linux)"
echo ""

# Start Jupyter Lab in background
nohup jupyter lab --ip=0.0.0.0 --port=8888 --no-browser --allow-root --NotebookApp.token='' --NotebookApp.password='' > /tmp/jupyter-lab.log 2>&1 &
JUPYTER_PID=$!

echo "Jupyter Lab started (PID: $JUPYTER_PID)"
echo "Logs: tail -f /tmp/jupyter-lab.log"

# Wait for server to be ready
sleep 5

# Open browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:8888 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Jupyter Lab running at http://localhost:8888"
`,
		Files: map[string]string{}, // No files needed - this is just for the startup script
	}

	// Jupyter Financial Analysis - Notebooks Repository
	// This entry only provides files for the notebooks code repository
	// Startup script comes from the main "jupyter-financial-analysis" entry
	s.sampleProjects["jupyter-notebooks"] = &SampleProjectCode{
		ID:           "jupyter-notebooks",
		Name:         "Jupyter Financial Analysis - Notebooks",
		Description:  "Jupyter notebooks for financial analysis with S&P 500 data",
		Technologies: []string{"Python", "Jupyter", "Pandas", "Finance"},
		Language:     "python",
		GitIgnore: `# Jupyter
.ipynb_checkpoints/
*.ipynb_checkpoints

# Python
__pycache__/
*.pyc
*.pyo
.venv/
venv/

# Output files
*.html
*.png
*.pdf
`,
		Files: map[string]string{
			"README.md": `# Jupyter Financial Analysis - Notebooks

This repository contains Jupyter notebooks for financial data analysis.

## Quick Start

The agent can run notebooks using ipynb commands from the terminal:

` + "```bash" + `
# Execute notebook and save results in-place
jupyter nbconvert --execute --to notebook --inplace portfolio_analysis.ipynb

# Execute and export to HTML for viewing
jupyter nbconvert --execute --to html portfolio_analysis.ipynb

# View the HTML output in browser
open portfolio_analysis.html  # macOS
xdg-open portfolio_analysis.html  # Linux
` + "```" + `

## Notebooks

- **portfolio_analysis.ipynb**: Microsoft stock returns analysis with pyforest library

## Related Repositories

- **pyforest**: Python library for financial calculations (attach as second repository to project)
`,
			"AGENT_INSTRUCTIONS.md": `# Agent Instructions for Jupyter Workflow

## Running Notebooks from Command Line

Execute Jupyter notebooks without opening the browser:

` + "```bash" + `
# Method 1: Execute and save results in the notebook
jupyter nbconvert --execute --to notebook --inplace portfolio_analysis.ipynb

# Method 2: Execute and generate HTML output
jupyter nbconvert --execute --to html portfolio_analysis.ipynb

# Method 3: Check for errors only
jupyter nbconvert --execute --to notebook --stdout portfolio_analysis.ipynb > /dev/null
` + "```" + `

## Viewing Results

After running a notebook:
1. The .ipynb file will contain cell outputs if you used --inplace
2. The .html file can be viewed in a browser
3. Use Bash tool: ` + "`xdg-open portfolio_analysis.html`" + ` to open in browser

## Iterative Development Workflow

1. **Read** the notebook using Read tool on .ipynb file
2. **Modify** cells using NotebookEdit tool
3. **Execute**: ` + "`jupyter nbconvert --execute --to html portfolio_analysis.ipynb`" + `
4. **View** results by reading the .html file or opening in browser
5. **Iterate** based on results

## Working with PyForest Library

The pyforest library should be in a sibling repository (../pyforest):
1. Edit files in ../pyforest/pyforest/ directory
2. Library is installed as editable (` + "`pip install -e ../pyforest`" + `)
3. Changes are immediately available after kernel restart
4. Re-run notebook cells to pick up changes

## Example: Adding Multiple Tickers

1. Read portfolio_analysis.ipynb
2. Find the cell with ` + "`ticker = 'MSFT'`" + `
3. Edit to download multiple tickers in a loop
4. Run: ` + "`jupyter nbconvert --execute --to html portfolio_analysis.ipynb`" + `
5. Open portfolio_analysis.html to verify charts show all tickers
6. Commit changes to git
`,
			"requirements.txt": `jupyterlab>=4.3.0
pandas>=2.2.0
numpy>=2.0.0
matplotlib>=3.9.0
yfinance>=0.2.40
scipy>=1.14.0
seaborn>=0.13.2
`,
			"portfolio_analysis.ipynb": `{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# Microsoft Portfolio Returns Analysis\n",
    "\n",
    "This notebook calculates portfolio returns for Microsoft (MSFT) over different date ranges.\n",
    "\n",
    "**Note**: This notebook uses the pyforest library from the separate pyforest repository."
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import pandas as pd\n",
    "import numpy as np\n",
    "import yfinance as yf\n",
    "import matplotlib.pyplot as plt\n",
    "from datetime import datetime, timedelta\n",
    "\n",
    "# Import pyforest library (install from ../pyforest with: pip install -e ../pyforest)\n",
    "try:\n",
    "    from pyforest import calculate_returns, calculate_cumulative_returns, calculate_sharpe_ratio\n",
    "    print(\"PyForest library imported successfully\")\n",
    "except ImportError:\n",
    "    print(\"WARNING: PyForest library not found. Install with: pip install -e ../pyforest\")\n",
    "    print(\"Falling back to basic pandas calculations\")\n",
    "\n",
    "# Set display options\n",
    "pd.set_option('display.max_columns', None)\n",
    "pd.set_option('display.width', None)\n",
    "\n",
    "print(\"Libraries imported successfully\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Download Microsoft stock data\n",
    "ticker = 'MSFT'\n",
    "print(f\"Downloading {ticker} data...\")\n",
    "\n",
    "# Get data for last 5 years\n",
    "end_date = datetime.now()\n",
    "start_date = end_date - timedelta(days=5*365)\n",
    "\n",
    "msft = yf.download(ticker, start=start_date, end=end_date, progress=False)\n",
    "print(f\"Downloaded {len(msft)} days of data\")\n",
    "msft.head()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Calculate returns using pyforest library if available, otherwise use pandas\n",
    "try:\n",
    "    msft['Daily_Return'] = calculate_returns(msft['Close'])\n",
    "    msft['Cumulative_Return'] = calculate_cumulative_returns(msft['Daily_Return'])\n",
    "    sharpe = calculate_sharpe_ratio(msft['Daily_Return'])\n",
    "except NameError:\n",
    "    # Fallback if pyforest not imported\n",
    "    msft['Daily_Return'] = msft['Close'].pct_change()\n",
    "    msft['Cumulative_Return'] = (1 + msft['Daily_Return']).cumprod() - 1\n",
    "    sharpe = (msft['Daily_Return'].mean() / msft['Daily_Return'].std()) * np.sqrt(252)\n",
    "\n",
    "print(\"\\nReturn Statistics:\")\n",
    "print(f\"Total Return: {msft['Cumulative_Return'].iloc[-1]:.2%}\")\n",
    "print(f\"Average Daily Return: {msft['Daily_Return'].mean():.4%}\")\n",
    "print(f\"Daily Return Std Dev: {msft['Daily_Return'].std():.4%}\")\n",
    "print(f\"Sharpe Ratio (annualized): {sharpe:.2f}\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Analyze returns over different date ranges\n",
    "date_ranges = [\n",
    "    ('1 Month', 30),\n",
    "    ('3 Months', 90),\n",
    "    ('6 Months', 180),\n",
    "    ('1 Year', 365),\n",
    "    ('2 Years', 730),\n",
    "    ('5 Years', 1825),\n",
    "]\n",
    "\n",
    "results = []\n",
    "for label, days in date_ranges:\n",
    "    if len(msft) >= days:\n",
    "        period_data = msft.iloc[-days:]\n",
    "        total_return = (period_data['Close'].iloc[-1] / period_data['Close'].iloc[0] - 1)\n",
    "        results.append({\n",
    "            'Period': label,\n",
    "            'Days': days,\n",
    "            'Return': total_return,\n",
    "            'Start_Price': period_data['Close'].iloc[0],\n",
    "            'End_Price': period_data['Close'].iloc[-1],\n",
    "        })\n",
    "\n",
    "returns_df = pd.DataFrame(results)\n",
    "print(\"\\nReturns by Period:\")\n",
    "print(returns_df.to_string(index=False))"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Plot cumulative returns\n",
    "plt.figure(figsize=(14, 7))\n",
    "plt.plot(msft.index, msft['Cumulative_Return'] * 100, linewidth=2, color='#1f77b4')\n",
    "plt.title(f'{ticker} Cumulative Returns Over Time', fontsize=16, fontweight='bold')\n",
    "plt.xlabel('Date', fontsize=12)\n",
    "plt.ylabel('Cumulative Return (%)', fontsize=12)\n",
    "plt.grid(True, alpha=0.3)\n",
    "plt.tight_layout()\n",
    "plt.savefig('msft_returns.png', dpi=150, bbox_inches='tight')\n",
    "plt.show()\n",
    "\n",
    "print(f\"\\nCurrent Price: ${msft['Close'].iloc[-1]:.2f}\")\n",
    "print(f\"52-Week High: ${msft['Close'].iloc[-252:].max():.2f}\")\n",
    "print(f\"52-Week Low: ${msft['Close'].iloc[-252:].min():.2f}\")\n",
    "print(\"\\nChart saved to msft_returns.png\")"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "codemirror_mode": {
    "name": "ipython",
    "version": 3
   },
   "file_extension": ".py",
   "mimetype": "text/x-python",
   "name": "python",
   "nbconvert_exporter": "python",
   "pygments_lexer": "ipython3",
   "version": "3.11.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}`,
		},
	}

	// PyForest Library Repository
	// This entry only provides files for the pyforest library code repository
	// Startup script comes from the main "jupyter-financial-analysis" entry
	s.sampleProjects["pyforest-library"] = &SampleProjectCode{
		ID:           "pyforest-library",
		Name:         "PyForest Financial Library",
		Description:  "Python library for financial analysis and portfolio management",
		Technologies: []string{"Python", "Pandas", "NumPy"},
		Language:     "python",
		GitIgnore: `# Python
__pycache__/
*.pyc
*.pyo
*.egg-info/
dist/
build/
.pytest_cache/
`,
		Files: map[string]string{
			"README.md": `# PyForest - Financial Analysis Library

A Python library for financial data analysis and portfolio management.

## Installation

` + "```bash" + `
pip install -e .
` + "```" + `

## Modules

- **returns.py**: Return calculations (simple, log, cumulative, Sharpe ratio)
- **portfolio.py**: Portfolio management and optimization
- **indicators.py**: Technical indicators (to be implemented by agent)

## Usage

` + "```python" + `
from pyforest import calculate_returns, Portfolio

# Calculate returns
returns = calculate_returns(price_series)

# Create portfolio
portfolio = Portfolio(['MSFT', 'AAPL'], weights=[0.6, 0.4])
` + "```" + `

## Agent Development

This library is designed to be extended with new financial analysis functions.
Use the notebook in the jupyter-notebooks repository to test new features.
`,
			"setup.py": `from setuptools import setup, find_packages

setup(
    name="pyforest",
    version="0.1.0",
    description="Financial analysis library for portfolio returns calculation",
    author="Helix Agent",
    packages=find_packages(),
    install_requires=[
        "pandas>=2.0.0",
        "numpy>=1.24.0",
    ],
    python_requires=">=3.8",
)
`,
			"pyforest/__init__.py": `"""
PyForest - Financial Analysis Library
"""

from .returns import calculate_returns, calculate_cumulative_returns, calculate_sharpe_ratio
from .portfolio import Portfolio, PortfolioOptimizer

__version__ = "0.1.0"
__all__ = [
    "calculate_returns",
    "calculate_cumulative_returns",
    "calculate_sharpe_ratio",
    "Portfolio",
    "PortfolioOptimizer",
]
`,
			"pyforest/returns.py": `"""
Return calculation utilities for financial analysis
"""

import pandas as pd
import numpy as np


def calculate_returns(prices: pd.Series, method='simple') -> pd.Series:
    """
    Calculate returns from a price series.

    Parameters:
    -----------
    prices : pd.Series
        Time series of prices
    method : str
        'simple' for simple returns, 'log' for logarithmic returns

    Returns:
    --------
    pd.Series
        Series of returns
    """
    if method == 'simple':
        return prices.pct_change()
    elif method == 'log':
        return np.log(prices / prices.shift(1))
    else:
        raise ValueError(f"Unknown method: {method}")


def calculate_cumulative_returns(returns: pd.Series) -> pd.Series:
    """
    Calculate cumulative returns from a returns series.
    """
    return (1 + returns).cumprod() - 1


def calculate_sharpe_ratio(returns: pd.Series, risk_free_rate: float = 0.02, periods_per_year: int = 252) -> float:
    """
    Calculate annualized Sharpe ratio.
    """
    excess_returns = returns - (risk_free_rate / periods_per_year)
    return (excess_returns.mean() / excess_returns.std()) * np.sqrt(periods_per_year)
`,
			"pyforest/portfolio.py": `"""
Portfolio management and optimization utilities
"""

import pandas as pd
import numpy as np
from typing import List, Dict


class Portfolio:
    """
    Portfolio class for managing multiple assets.
    """

    def __init__(self, tickers: List[str], weights: List[float] = None):
        self.tickers = tickers
        if weights is None:
            self.weights = np.array([1.0 / len(tickers)] * len(tickers))
        else:
            self.weights = np.array(weights)
            if not np.isclose(self.weights.sum(), 1.0):
                raise ValueError("Weights must sum to 1.0")

    def calculate_portfolio_returns(self, returns_df: pd.DataFrame) -> pd.Series:
        """Calculate portfolio returns from individual asset returns."""
        return (returns_df[self.tickers] * self.weights).sum(axis=1)

    def calculate_metrics(self, returns: pd.Series, periods_per_year: int = 252) -> Dict:
        """Calculate portfolio performance metrics."""
        metrics = {
            'mean_return': returns.mean() * periods_per_year,
            'volatility': returns.std() * np.sqrt(periods_per_year),
            'sharpe_ratio': (returns.mean() / returns.std()) * np.sqrt(periods_per_year),
        }

        # Maximum drawdown
        cumulative = (1 + returns).cumprod()
        running_max = cumulative.expanding().max()
        drawdown = (cumulative - running_max) / running_max
        metrics['max_drawdown'] = drawdown.min()

        return metrics


class PortfolioOptimizer:
    """Portfolio optimization using mean-variance."""

    def __init__(self, returns_df: pd.DataFrame):
        self.returns_df = returns_df
        self.mean_returns = returns_df.mean()
        self.cov_matrix = returns_df.cov()

    def optimize_sharpe(self) -> Dict:
        """Find portfolio weights that maximize Sharpe ratio."""
        # TODO: Implement with scipy.optimize
        # For now, return equal weights
        n_assets = len(self.returns_df.columns)
        weights = np.array([1.0 / n_assets] * n_assets)

        return {
            'weights': dict(zip(self.returns_df.columns, weights)),
            'expected_return': np.dot(weights, self.mean_returns),
            'expected_volatility': np.sqrt(np.dot(weights.T, np.dot(self.cov_matrix, weights))),
        }
`,
		},
	}

	// Data Platform API Migration Suite
	s.sampleProjects["data-platform-api-migration"] = &SampleProjectCode{
		ID:           "data-platform-api-migration",
		Name:         "Data Platform API Migration Suite",
		Description:  "Migrate data pipeline APIs from legacy infrastructure to modern data platform",
		Technologies: []string{"Python", "FastAPI", "Apache Airflow", "Pandas", "SQLAlchemy"},
		Language:     "python",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Add user's local bin to PATH for pip-installed executables
if ! grep -q 'export PATH="$HOME/.local/bin:$PATH"' ~/.bashrc; then
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
    echo "✅ Added ~/.local/bin to PATH in ~/.bashrc"
fi
export PATH="$HOME/.local/bin:$PATH"

# Install Python 3 and pip if not present
if ! command -v python3 &> /dev/null; then
    echo "📦 Installing Python 3..."
    sudo apt-get update
    sudo apt-get install -y python3 python3-pip python3-venv
fi

if ! command -v pip3 &> /dev/null; then
    echo "📦 Installing pip3..."
    sudo apt-get update
    sudo apt-get install -y python3-pip
fi

# Install requirements
if [ -f "requirements.txt" ]; then
    echo "📦 Installing Python dependencies..."
    pip3 install --break-system-packages -r requirements.txt
fi

echo "🚀 Starting FastAPI server in background..."
nohup uvicorn main:app --host 0.0.0.0 --port 8000 --reload > /tmp/fastapi-server.log 2>&1 &
API_PID=$!

echo "FastAPI server started (PID: $API_PID)"
echo "Logs: tail -f /tmp/fastapi-server.log"

# Wait for server to be ready
sleep 3

# Open browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:8000/docs > /dev/null 2>&1 &
fi

echo "✅ Startup complete - API docs at http://localhost:8000/docs"
`,
		GitIgnore: `# Python
__pycache__/
*.pyc
*.pyo
.venv/
venv/

# Database
*.db
*.sqlite

# Environment
.env
.env.local
`,
		Files: map[string]string{
			"README.md": `# Data Platform API Migration Suite

Migrate legacy data pipeline APIs to modern data platform.

## Architecture

- **FastAPI** - Modern API framework for legacy API simulation
- **Apache Airflow** - DAG orchestration for migration workflows
- **Pandas** - Data transformation and validation
- **SQLAlchemy** - Database abstraction for source and target systems

## Migration Workflow

1. Legacy API extraction (FastAPI endpoints simulate legacy systems)
2. Schema mapping (YAML configuration)
3. Data transformation (Pandas pipelines)
4. Quality validation (automated checks)
5. Load to target warehouse (SQLAlchemy)
6. Lineage documentation (automated generation)

## Quick Start

` + "```bash" + `
pip install -r requirements.txt
uvicorn main:app --reload
` + "```" + `

Open http://localhost:8000/docs for API documentation.
`,
			"requirements.txt": `fastapi>=0.115.0
uvicorn[standard]>=0.34.0
pandas>=2.2.0
sqlalchemy>=2.0.0
pydantic>=2.10.0
pyyaml>=6.0
httpx>=0.28.0
# apache-airflow>=2.10.0  # Optional - install separately if you need to run the DAG
`,
			"main.py": `from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Dict, Any
import pandas as pd
from datetime import datetime

app = FastAPI(title="Data Platform API Migration Suite")

# Sample legacy API responses (simulating multiple legacy APIs)
class LegacyCustomerResponse(BaseModel):
    customer_id: int
    name: str
    email: str
    created_date: str
    account_balance: float
    status: str

class LegacyTransactionResponse(BaseModel):
    transaction_id: int
    customer_id: int
    amount: float
    transaction_type: str
    timestamp: str

@app.get("/")
def root():
    return {
        "message": "Data Platform API Migration Suite",
        "legacy_apis": ["customers", "transactions", "products"],
        "total_apis_to_migrate": "100+",
        "docs": "/docs"
    }

@app.get("/api/legacy/customers", response_model=List[LegacyCustomerResponse])
def get_legacy_customers():
    """Legacy customer API endpoint - simulates one of many APIs to migrate"""
    return [
        {
            "customer_id": 1,
            "name": "Alice Johnson",
            "email": "alice@example.com",
            "created_date": "2020-01-15",
            "account_balance": 50000.00,
            "status": "active"
        },
        {
            "customer_id": 2,
            "name": "Bob Smith",
            "email": "bob@example.com",
            "created_date": "2021-06-22",
            "account_balance": 75000.00,
            "status": "active"
        },
    ]

@app.get("/api/legacy/transactions", response_model=List[LegacyTransactionResponse])
def get_legacy_transactions():
    """Legacy transaction API endpoint"""
    return [
        {
            "transaction_id": 101,
            "customer_id": 1,
            "amount": 1500.00,
            "transaction_type": "deposit",
            "timestamp": "2024-01-10T14:30:00Z"
        },
        {
            "transaction_id": 102,
            "customer_id": 2,
            "amount": -500.00,
            "transaction_type": "withdrawal",
            "timestamp": "2024-01-11T09:15:00Z"
        },
    ]

@app.get("/api/migration/status")
def migration_status():
    """Track migration progress"""
    return {
        "total_apis": 100,
        "migrated": 2,
        "in_progress": 5,
        "pending": 93,
        "last_updated": datetime.now().isoformat()
    }
`,
			"schema_mappings.yaml": `# Schema mappings from legacy APIs to modern data warehouse

customers:
  source: "/api/legacy/customers"
  target_table: "dim_customer"
  mappings:
    customer_id: id
    name: full_name
    email: email_address
    created_date: onboarded_at
    account_balance: current_balance
    status: account_status
  transformations:
    onboarded_at: "datetime_conversion"
    current_balance: "decimal_precision_2"

transactions:
  source: "/api/legacy/transactions"
  target_table: "fact_transaction"
  mappings:
    transaction_id: transaction_key
    customer_id: customer_key
    amount: transaction_amount
    transaction_type: transaction_category
    timestamp: transaction_datetime
  transformations:
    transaction_datetime: "datetime_conversion"
    transaction_amount: "decimal_precision_2"
  dependencies:
    - customers  # Must migrate customers first
`,
			"dags/migration_dag.py": `from airflow import DAG
from airflow.operators.python import PythonOperator
from datetime import datetime, timedelta
import requests
import pandas as pd

default_args = {
    'owner': 'data-platform-team',
    'depends_on_past': False,
    'start_date': datetime(2024, 1, 1),
    'email_on_failure': False,
    'email_on_retry': False,
    'retries': 3,
    'retry_delay': timedelta(minutes=5),
}

dag = DAG(
    'api_migration_dag',
    default_args=default_args,
    description='Orchestrate migration of legacy APIs to modern platform',
    schedule_interval=timedelta(days=1),
    catchup=False,
)

def extract_legacy_api(api_endpoint):
    """Extract data from legacy API"""
    response = requests.get(f"http://localhost:8000{api_endpoint}")
    return response.json()

def transform_and_load(data, schema_mapping):
    """Transform data according to schema mapping and load to warehouse"""
    df = pd.DataFrame(data)
    # Apply transformations based on schema_mapping
    # Load to target data warehouse
    print(f"Transformed and loaded {len(df)} records")
    return True

def validate_migration(source_count, target_count):
    """Validate migration completed successfully"""
    if source_count != target_count:
        raise ValueError(f"Data loss detected: {source_count} != {target_count}")
    return True

# Task: Extract customers
extract_customers = PythonOperator(
    task_id='extract_customers',
    python_callable=extract_legacy_api,
    op_args=['/api/legacy/customers'],
    dag=dag,
)

# Task: Extract transactions (depends on customers)
extract_transactions = PythonOperator(
    task_id='extract_transactions',
    python_callable=extract_legacy_api,
    op_args=['/api/legacy/transactions'],
    dag=dag,
)

# Set dependencies
extract_customers >> extract_transactions
`,
		},
	}

	// Portfolio Management System (.NET)
	s.sampleProjects["portfolio-management-dotnet"] = &SampleProjectCode{
		ID:           "portfolio-management-dotnet",
		Name:         "Portfolio Management System (.NET)",
		Description:  "Production-grade portfolio management and trade execution system",
		Technologies: []string{"C#", ".NET 8", "Entity Framework Core", "xUnit"},
		Language:     "csharp",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Install wget if not present
if ! command -v wget &> /dev/null; then
    echo "📦 Installing wget..."
    sudo apt-get update
    sudo apt-get install -y wget
fi

# Install .NET SDK 8.0 if not present
if ! command -v dotnet &> /dev/null; then
    echo "📦 Installing .NET SDK 8.0..."
    wget https://dot.net/v1/dotnet-install.sh -O /tmp/dotnet-install.sh
    chmod +x /tmp/dotnet-install.sh
    /tmp/dotnet-install.sh --channel 8.0 --install-dir $HOME/.dotnet

    # Add to bashrc
    if ! grep -q 'export PATH="$HOME/.dotnet:$PATH"' ~/.bashrc; then
        echo 'export PATH="$HOME/.dotnet:$PATH"' >> ~/.bashrc
        echo 'export DOTNET_ROOT="$HOME/.dotnet"' >> ~/.bashrc
        echo "✅ Added .dotnet to PATH in ~/.bashrc"
    fi
fi

# Ensure dotnet is in PATH
export PATH="$HOME/.dotnet:$PATH"
export DOTNET_ROOT="$HOME/.dotnet"

# Install NATS server if not present
if ! command -v nats-server &> /dev/null; then
    echo "📦 Installing NATS server..."
    NATS_VERSION="v2.10.22"
    wget "https://github.com/nats-io/nats-server/releases/download/${NATS_VERSION}/nats-server-${NATS_VERSION}-linux-amd64.tar.gz" -O /tmp/nats-server.tar.gz
    tar -xzf /tmp/nats-server.tar.gz -C /tmp
    sudo mv "/tmp/nats-server-${NATS_VERSION}-linux-amd64/nats-server" /usr/local/bin/
    rm -rf /tmp/nats-server*
    echo "✅ NATS server installed"
fi

# Start NATS server in background
echo "🚀 Starting NATS server..."
nohup nats-server > /tmp/nats-server.log 2>&1 &
NATS_PID=$!
echo "NATS server started (PID: $NATS_PID) on port 4222"
sleep 2

echo "🏗️  Restoring .NET dependencies..."
dotnet restore

echo "🔨 Building solution..."
dotnet build --no-restore

echo "🚀 Starting API server in background..."
nohup dotnet run --project PortfolioManagement.API --no-build > /tmp/dotnet-api.log 2>&1 &
API_PID=$!

echo "API server started (PID: $API_PID)"
echo "Logs: tail -f /tmp/dotnet-api.log"

# Wait for server to be ready
sleep 5

# Open browser to Swagger docs
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:5000/swagger > /dev/null 2>&1 &
fi

echo "✅ Startup complete - API running at http://localhost:5000/swagger"
`,
		GitIgnore: `# .NET
bin/
obj/
*.user
*.suo

# Environment
.env
.env.local

# Database
*.db
*.sqlite
`,
		Files: map[string]string{
			"README.md": `# Portfolio Management System (.NET)

Production-grade portfolio and trade execution system.

## Architecture

Clean Architecture with:
- **Domain** - Entities, value objects, domain services
- **Application** - Use cases, DTOs, interfaces
- **Infrastructure** - Database, external services, messaging
- **API** - Controllers, middleware, configuration

## Technology Stack

- .NET 8
- Entity Framework Core
- SignalR (real-time updates)
- NATS Messaging (async order processing, runs locally)
- xUnit (testing)

## Architecture Notes

- NATS server runs locally (installed and started automatically)
- Uses System.Threading.Channels for in-process queues
- Can be swapped for cloud messaging (Azure Service Bus, AWS SQS) in production

## Quick Start

` + "```bash" + `
dotnet restore
dotnet build
dotnet run --project PortfolioManagement.API
` + "```" + `

API docs: http://localhost:5000/swagger
`,
			"PortfolioManagement.sln": `
Microsoft Visual Studio Solution File, Format Version 12.00
# Visual Studio Version 17
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "PortfolioManagement.Domain", "PortfolioManagement.Domain\\PortfolioManagement.Domain.csproj", "{11111111-1111-1111-1111-111111111111}"
EndProject
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "PortfolioManagement.Application", "PortfolioManagement.Application\\PortfolioManagement.Application.csproj", "{22222222-2222-2222-2222-222222222222}"
EndProject
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "PortfolioManagement.Infrastructure", "PortfolioManagement.Infrastructure\\PortfolioManagement.Infrastructure.csproj", "{33333333-3333-3333-3333-333333333333}"
EndProject
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "PortfolioManagement.API", "PortfolioManagement.API\\PortfolioManagement.API.csproj", "{44444444-4444-4444-4444-444444444444}"
EndProject
Global
	GlobalSection(SolutionConfigurationPlatforms) = preSolution
		Debug|Any CPU = Debug|Any CPU
		Release|Any CPU = Release|Any CPU
	EndGlobalSection
	GlobalSection(ProjectConfigurationPlatforms) = postSolution
		{11111111-1111-1111-1111-111111111111}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{11111111-1111-1111-1111-111111111111}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{22222222-2222-2222-2222-222222222222}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{22222222-2222-2222-2222-222222222222}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{33333333-3333-3333-3333-333333333333}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{33333333-3333-3333-3333-333333333333}.Debug|Any CPU.Build.0 = Debug|Any CPU
		{44444444-4444-4444-4444-444444444444}.Debug|Any CPU.ActiveCfg = Debug|Any CPU
		{44444444-4444-4444-4444-444444444444}.Debug|Any CPU.Build.0 = Debug|Any CPU
	EndGlobalSection
EndGlobal
`,
			"PortfolioManagement.Domain/PortfolioManagement.Domain.csproj": `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
  </PropertyGroup>
</Project>
`,
			"PortfolioManagement.Domain/Entities/Portfolio.cs": `namespace PortfolioManagement.Domain.Entities;

public class Portfolio
{
    public Guid Id { get; private set; }
    public string Name { get; private set; }
    public decimal CashBalance { get; private set; }
    public List<Position> Positions { get; private set; } = new();
    public DateTime CreatedAt { get; private set; }
    public DateTime UpdatedAt { get; private set; }

    public Portfolio(string name, decimal initialCash)
    {
        Id = Guid.NewGuid();
        Name = name;
        CashBalance = initialCash;
        CreatedAt = DateTime.UtcNow;
        UpdatedAt = DateTime.UtcNow;
    }

    public decimal CalculateTotalValue(Dictionary<string, decimal> currentPrices)
    {
        var positionsValue = Positions.Sum(p =>
            currentPrices.TryGetValue(p.Symbol, out var price)
                ? p.Quantity * price
                : 0);
        return CashBalance + positionsValue;
    }

    public decimal CalculatePnL(Dictionary<string, decimal> currentPrices)
    {
        return Positions.Sum(p =>
        {
            if (currentPrices.TryGetValue(p.Symbol, out var currentPrice))
                return (currentPrice - p.CostBasis) * p.Quantity;
            return 0;
        });
    }
}

public class Position
{
    public string Symbol { get; set; } = string.Empty;
    public decimal Quantity { get; set; }
    public decimal CostBasis { get; set; }
    public DateTime AcquiredAt { get; set; }
}
`,
			"PortfolioManagement.Application/PortfolioManagement.Application.csproj": `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
  </PropertyGroup>
  <ItemGroup>
    <ProjectReference Include="../PortfolioManagement.Domain/PortfolioManagement.Domain.csproj" />
  </ItemGroup>
</Project>
`,
			"PortfolioManagement.Infrastructure/PortfolioManagement.Infrastructure.csproj": `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="NATS.Client.Core" Version="2.4.0" />
    <ProjectReference Include="../PortfolioManagement.Domain/PortfolioManagement.Domain.csproj" />
    <ProjectReference Include="../PortfolioManagement.Application/PortfolioManagement.Application.csproj" />
  </ItemGroup>
</Project>
`,
			"PortfolioManagement.API/PortfolioManagement.API.csproj": `<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <Nullable>enable</Nullable>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Swashbuckle.AspNetCore" Version="6.8.1" />
    <ProjectReference Include="../PortfolioManagement.Application/PortfolioManagement.Application.csproj" />
    <ProjectReference Include="../PortfolioManagement.Domain/PortfolioManagement.Domain.csproj" />
  </ItemGroup>
</Project>
`,
			"PortfolioManagement.API/Program.cs": `using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddControllers();
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen();

var app = builder.Build();

if (app.Environment.IsDevelopment())
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

app.MapControllers();

app.Run("http://0.0.0.0:5000");
`,
			"PortfolioManagement.API/Controllers/PortfolioController.cs": `using Microsoft.AspNetCore.Mvc;
using PortfolioManagement.Domain.Entities;

namespace PortfolioManagement.API.Controllers;

[ApiController]
[Route("api/[controller]")]
public class PortfolioController : ControllerBase
{
    // In-memory storage for demo
    private static readonly Portfolio DemoPortfolio = new("Demo Portfolio", 100000m);

    [HttpGet]
    public IActionResult GetPortfolio()
    {
        return Ok(new
        {
            id = DemoPortfolio.Id,
            name = DemoPortfolio.Name,
            cashBalance = DemoPortfolio.CashBalance,
            positions = DemoPortfolio.Positions,
            totalValue = DemoPortfolio.CalculateTotalValue(new Dictionary<string, decimal>
            {
                { "AAPL", 150.00m },
                { "MSFT", 350.00m }
            })
        });
    }

    [HttpGet("pnl")]
    public IActionResult GetPnL()
    {
        var currentPrices = new Dictionary<string, decimal>
        {
            { "AAPL", 150.00m },
            { "MSFT", 350.00m }
        };

        return Ok(new
        {
            portfolioId = DemoPortfolio.Id,
            realizedPnL = 0m,
            unrealizedPnL = DemoPortfolio.CalculatePnL(currentPrices),
            totalPnL = DemoPortfolio.CalculatePnL(currentPrices)
        });
    }
}
`,
		},
	}

	// Research Analysis Toolkit
	s.sampleProjects["research-analysis-toolkit"] = &SampleProjectCode{
		ID:           "research-analysis-toolkit",
		Name:         "Research Analysis Toolkit (PyForest)",
		Description:  "Financial research notebooks using PyForest for backtesting and optimization",
		Technologies: []string{"Python", "Jupyter", "Pandas", "NumPy", "PyForest"},
		Language:     "python",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Add user's local bin to PATH
if ! grep -q 'export PATH="$HOME/.local/bin:$PATH"' ~/.bashrc; then
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
    echo "✅ Added ~/.local/bin to PATH in ~/.bashrc"
fi
export PATH="$HOME/.local/bin:$PATH"

# Install Python 3 and pip if not present
if ! command -v python3 &> /dev/null; then
    echo "📦 Installing Python 3..."
    sudo apt-get update
    sudo apt-get install -y python3 python3-pip python3-venv
fi

if ! command -v pip3 &> /dev/null; then
    sudo apt-get install -y python3-pip
fi

# Install requirements
if [ -f "requirements.txt" ]; then
    echo "📦 Installing Python dependencies..."
    pip3 install --break-system-packages -r requirements.txt
fi

echo "🚀 Starting Jupyter Lab in background..."
nohup jupyter lab --ip=0.0.0.0 --port=8888 --no-browser --allow-root --NotebookApp.token='' --NotebookApp.password='' > /tmp/jupyter-lab.log 2>&1 &
JUPYTER_PID=$!

echo "Jupyter Lab started (PID: $JUPYTER_PID)"
echo "Logs: tail -f /tmp/jupyter-lab.log"

sleep 5

if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:8888 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Jupyter Lab running at http://localhost:8888"
`,
		GitIgnore: `# Jupyter
.ipynb_checkpoints/
*.ipynb_checkpoints

# Python
__pycache__/
*.pyc
.venv/

# Output
*.html
*.png
*.pdf
`,
		Files: map[string]string{
			"README.md": `# Research Analysis Toolkit (PyForest)

Financial research using PyForest library.

## Notebooks

- **strategy_backtesting.ipynb** - SMA crossover strategy backtesting
- **portfolio_optimization.ipynb** - Mean-variance optimization
- **risk_factor_analysis.ipynb** - Factor decomposition

## PyForest Integration

This toolkit uses mock PyForest library for:
- Portfolio data access
- Return calculations
- Risk metrics
- Backtesting framework

## Quick Start

` + "```bash" + `
pip install -r requirements.txt
jupyter lab
` + "```" + `
`,
			"requirements.txt": `jupyterlab>=4.3.0
pandas>=2.2.0
numpy>=2.0.0
matplotlib>=3.9.0
scipy>=1.14.0
seaborn>=0.13.2
yfinance>=0.2.40
`,
			"strategy_backtesting.ipynb": `{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# SMA Crossover Strategy Backtesting\n",
    "\n",
    "Test simple moving average crossover strategy on historical data."
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import pandas as pd\n",
    "import numpy as np\n",
    "import yfinance as yf\n",
    "import matplotlib.pyplot as plt\n",
    "\n",
    "print(\"Libraries imported successfully\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Download sample data\n",
    "ticker = 'AAPL'\n",
    "data = yf.download(ticker, start='2020-01-01', end='2024-01-01', progress=False)\n",
    "print(f\"Downloaded {len(data)} days of data for {ticker}\")\n",
    "data.head()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Calculate SMAs\n",
    "data['SMA_50'] = data['Close'].rolling(window=50).mean()\n",
    "data['SMA_200'] = data['Close'].rolling(window=200).mean()\n",
    "\n",
    "# Generate signals: 1 when SMA50 > SMA200 (buy), -1 otherwise\n",
    "data['Signal'] = np.where(data['SMA_50'] > data['SMA_200'], 1, -1)\n",
    "data['Position'] = data['Signal'].diff()\n",
    "\n",
    "print(\"Strategy signals generated\")\n",
    "print(f\"Buy signals: {(data['Position'] == 2).sum()}\")\n",
    "print(f\"Sell signals: {(data['Position'] == -2).sum()}\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Plot strategy\n",
    "plt.figure(figsize=(14, 7))\n",
    "plt.plot(data.index, data['Close'], label='Price', linewidth=2)\n",
    "plt.plot(data.index, data['SMA_50'], label='SMA 50', alpha=0.7)\n",
    "plt.plot(data.index, data['SMA_200'], label='SMA 200', alpha=0.7)\n",
    "\n",
    "# Mark buy/sell signals\n",
    "buys = data[data['Position'] == 2]\n",
    "sells = data[data['Position'] == -2]\n",
    "plt.scatter(buys.index, buys['Close'], color='green', marker='^', s=100, label='Buy')\n",
    "plt.scatter(sells.index, sells['Close'], color='red', marker='v', s=100, label='Sell')\n",
    "\n",
    "plt.title('SMA Crossover Strategy', fontsize=16, fontweight='bold')\n",
    "plt.xlabel('Date')\n",
    "plt.ylabel('Price ($)')\n",
    "plt.legend()\n",
    "plt.grid(True, alpha=0.3)\n",
    "plt.tight_layout()\n",
    "plt.show()"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "name": "python",
   "version": "3.11.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}`,
		},
	}

	// Data Validation Toolkit
	s.sampleProjects["data-validation-toolkit"] = &SampleProjectCode{
		ID:           "data-validation-toolkit",
		Name:         "Data Validation Toolkit",
		Description:  "Compare data structures and validate migrations",
		Technologies: []string{"Python", "Jupyter", "Pandas", "Great Expectations"},
		Language:     "python",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Add user's local bin to PATH
if ! grep -q 'export PATH="$HOME/.local/bin:$PATH"' ~/.bashrc; then
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
fi
export PATH="$HOME/.local/bin:$PATH"

# Install Python
if ! command -v python3 &> /dev/null; then
    sudo apt-get update
    sudo apt-get install -y python3 python3-pip python3-venv
fi

if ! command -v pip3 &> /dev/null; then
    sudo apt-get install -y python3-pip
fi

# Install requirements
if [ -f "requirements.txt" ]; then
    echo "📦 Installing Python dependencies..."
    pip3 install --break-system-packages -r requirements.txt
fi

echo "🚀 Starting Jupyter Lab in background..."
nohup jupyter lab --ip=0.0.0.0 --port=8888 --no-browser --allow-root --NotebookApp.token='' --NotebookApp.password='' > /tmp/jupyter-lab.log 2>&1 &

sleep 5

if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:8888 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Jupyter Lab running at http://localhost:8888"
`,
		GitIgnore: `# Jupyter
.ipynb_checkpoints/
__pycache__/
*.pyc
.venv/
*.html
*.png
`,
		Files: map[string]string{
			"README.md": `# Data Validation Toolkit

Compare and validate data across systems.

## Features

- Data profiling and statistics
- Schema comparison
- Great Expectations integration
- Visual quality reports
- Row-level reconciliation

## Quick Start

` + "```bash" + `
pip install -r requirements.txt
jupyter lab
` + "```" + `
`,
			"requirements.txt": `jupyterlab>=4.3.0
pandas>=2.2.0
numpy>=2.0.0
matplotlib>=3.9.0
seaborn>=0.13.2
great_expectations>=1.2.0
`,
			"data_profiling.ipynb": `{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# Data Profiling and Comparison\n",
    "\n",
    "Analyze source and target datasets to identify differences."
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import pandas as pd\n",
    "import numpy as np\n",
    "import matplotlib.pyplot as plt\n",
    "\n",
    "print(\"Libraries imported\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Sample source data\n",
    "source_data = pd.DataFrame({\n",
    "    'id': [1, 2, 3, 4, 5],\n",
    "    'name': ['Alice', 'Bob', 'Charlie', 'Diana', 'Eve'],\n",
    "    'value': [100, 200, 150, 300, 250],\n",
    "    'category': ['A', 'B', 'A', 'C', 'B']\n",
    "})\n",
    "\n",
    "# Sample target data (with differences)\n",
    "target_data = pd.DataFrame({\n",
    "    'id': [1, 2, 3, 4],\n",
    "    'name': ['Alice', 'Bob', 'Charlie', 'Diana'],\n",
    "    'value': [100, 200, 150, 305],  # Different value for id=4\n",
    "    'category': ['A', 'B', 'A', 'C']\n",
    "})\n",
    "\n",
    "print(f\"Source rows: {len(source_data)}\")\n",
    "print(f\"Target rows: {len(target_data)}\")\n",
    "print(f\"Row count difference: {len(source_data) - len(target_data)}\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Profile source data\n",
    "print(\"Source Data Profile:\")\n",
    "print(source_data.describe())\n",
    "print(\"\\nTarget Data Profile:\")\n",
    "print(target_data.describe())"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}`,
		},
	}

	// Angular Analytics Dashboard
	s.sampleProjects["angular-analytics-dashboard"] = &SampleProjectCode{
		ID:           "angular-analytics-dashboard",
		Name:         "Multi-Tenant Analytics Dashboard",
		Description:  "Multi-tenant analytics dashboard with RBAC and real-time updates",
		Technologies: []string{"Angular", "TypeScript", "RxJS", "NgRx", "PrimeNG"},
		Language:     "typescript",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

# Fix ownership
sudo chown -R retro:retro .

echo "📦 Installing dependencies..."
npm install

echo "🚀 Starting Angular dev server in background..."
nohup npm start > /tmp/angular-dev.log 2>&1 &
DEV_PID=$!

echo "Angular dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/angular-dev.log"

sleep 10

if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:4200 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Dashboard running at http://localhost:4200"
`,
		GitIgnore: `node_modules/
dist/
.angular/
.env
`,
		Files: map[string]string{
			"README.md": `# Multi-Tenant Analytics Dashboard (Angular)

Enterprise analytics dashboard with multi-tenancy and RBAC.

## Features

- Multi-tenant context management
- Role-based access control
- Configurable dashboards with drag-and-drop
- Real-time WebSocket updates
- Data export (PDF/Excel)

## Technology Stack

- Angular 18
- NgRx (state management)
- PrimeNG (UI components)
- Chart.js (visualization)
- RxJS (reactive programming)

## Quick Start

` + "```bash" + `
npm install
npm start
` + "```" + `

Open http://localhost:4200
`,
			"package.json": `{
  "name": "angular-analytics-dashboard",
  "version": "1.0.0",
  "scripts": {
    "start": "ng serve --host 0.0.0.0",
    "build": "ng build",
    "test": "ng test"
  },
  "dependencies": {
    "@angular/animations": "^18.0.0",
    "@angular/common": "^18.0.0",
    "@angular/compiler": "^18.0.0",
    "@angular/core": "^18.0.0",
    "@angular/forms": "^18.0.0",
    "@angular/platform-browser": "^18.0.0",
    "@angular/platform-browser-dynamic": "^18.0.0",
    "@angular/router": "^18.0.0",
    "@ngrx/store": "^18.0.0",
    "@ngrx/effects": "^18.0.0",
    "primeng": "^18.0.0",
    "primeicons": "^7.0.0",
    "rxjs": "^7.8.0",
    "tslib": "^2.3.0",
    "zone.js": "^0.14.0",
    "chart.js": "^4.4.0",
    "ng2-charts": "^6.0.0"
  },
  "devDependencies": {
    "@angular-devkit/build-angular": "^18.0.0",
    "@angular/cli": "^18.0.0",
    "@angular/compiler-cli": "^18.0.0",
    "typescript": "~5.4.0"
  }
}`,
			"angular.json": `{
  "$schema": "./node_modules/@angular/cli/lib/config/schema.json",
  "version": 1,
  "newProjectRoot": "projects",
  "projects": {
    "angular-analytics-dashboard": {
      "projectType": "application",
      "root": "",
      "sourceRoot": "src",
      "architect": {
        "build": {
          "builder": "@angular-devkit/build-angular:application",
          "options": {
            "outputPath": "dist",
            "index": "src/index.html",
            "browser": "src/main.ts",
            "tsConfig": "tsconfig.app.json",
            "styles": ["src/styles.css"]
          }
        },
        "serve": {
          "builder": "@angular-devkit/build-angular:dev-server",
          "options": {
            "buildTarget": "angular-analytics-dashboard:build"
          }
        }
      }
    }
  }
}`,
			"src/main.ts": `import { bootstrapApplication } from '@angular/platform-browser';
import { AppComponent } from './app/app.component';

bootstrapApplication(AppComponent)
  .catch(err => console.error(err));
`,
			"src/app/app.component.ts": `import { Component } from '@angular/core';

@Component({
  selector: 'app-root',
  standalone: true,
  template: '<div class="dashboard-container"><h1>Multi-Tenant Analytics Dashboard</h1><p>Enterprise analytics with RBAC and real-time updates</p></div>',
  styles: ['.dashboard-container { padding: 20px; text-align: center; }']
})
export class AppComponent {
  title = 'Analytics Dashboard';
}
`,
			"src/index.html": `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Analytics Dashboard</title>
  <base href="/">
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <app-root></app-root>
</body>
</html>`,
			"src/styles.css": `/* Global styles */
body {
  margin: 0;
  font-family: system-ui, -apple-system, sans-serif;
}
`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": false,
    "module": "ES2022",
    "lib": ["ES2022", "dom"],
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  }
}`,
			"tsconfig.app.json": `{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "outDir": "./out-tsc/app",
    "types": []
  },
  "files": [
    "src/main.ts"
  ]
}`,
		},
	}

	// Angular Version Migration (15 → 18)
	s.sampleProjects["angular-version-migration"] = &SampleProjectCode{
		ID:           "angular-version-migration",
		Name:         "Angular Version Migration (15 → 18)",
		Description:  "Migrate Angular 15 app to Angular 18 with standalone components",
		Technologies: []string{"Angular", "TypeScript", "Migration"},
		Language:     "typescript",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Install Node.js if not present
if ! command -v node &> /dev/null; then
    echo "📦 Installing Node.js..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
    echo "✅ Node.js $(node --version) installed"
fi

# Fix ownership
sudo chown -R retro:retro .

echo "📦 Installing Angular 15 dependencies (pre-migration)..."
npm install

echo "🚀 Starting Angular dev server in background..."
nohup npm start > /tmp/angular-dev.log 2>&1 &
DEV_PID=$!

echo "Angular dev server started (PID: $DEV_PID)"
echo "Logs: tail -f /tmp/angular-dev.log"

sleep 10

if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:4200 > /dev/null 2>&1 &
fi

echo "✅ Startup complete - Angular 15 app running at http://localhost:4200"
echo ""
echo "📋 Migration Tasks:"
echo "  1. Analyze breaking changes for Angular 18"
echo "  2. Update dependencies to Angular 18"
echo "  3. Migrate to standalone components"
echo "  4. Update routing to provideRouter"
echo "  5. Test thoroughly"
`,
		GitIgnore: `node_modules/
dist/
.angular/
.env
`,
		Files: map[string]string{
			"README.md": `# Angular Version Migration (15 → 18)

Migrate from Angular 15 to Angular 18 with modern standalone components.

## Current State: Angular 15

This is an Angular 15 application using:
- NgModule-based architecture
- Old RouterModule.forRoot() pattern
- Component declarations in modules
- HttpClientModule imports

## Target State: Angular 18

Migrate to:
- Standalone components
- provideRouter() and bootstrapApplication()
- Functional guards and interceptors
- Modern dependency injection patterns

## Migration Strategy

1. Analyze breaking changes
2. Update dependencies
3. Convert to standalone components
4. Modernize routing
5. Fix deprecated APIs
6. Comprehensive testing

## Quick Start

` + "```bash" + `
npm install
npm start
` + "```" + `

Current app (Angular 15): http://localhost:4200
`,
			"package.json": `{
  "name": "angular-version-migration",
  "version": "15.0.0",
  "scripts": {
    "start": "ng serve --host 0.0.0.0",
    "build": "ng build",
    "test": "ng test"
  },
  "dependencies": {
    "@angular/animations": "^15.2.10",
    "@angular/common": "^15.2.10",
    "@angular/compiler": "^15.2.10",
    "@angular/core": "^15.2.10",
    "@angular/forms": "^15.2.10",
    "@angular/platform-browser": "^15.2.10",
    "@angular/platform-browser-dynamic": "^15.2.10",
    "@angular/router": "^15.2.10",
    "rxjs": "^7.5.0",
    "tslib": "^2.3.0",
    "zone.js": "^0.12.0"
  },
  "devDependencies": {
    "@angular-devkit/build-angular": "^15.2.10",
    "@angular/cli": "^15.2.10",
    "@angular/compiler-cli": "^15.2.10",
    "typescript": "~4.9.0"
  }
}`,
			"angular.json": `{
  "$schema": "./node_modules/@angular/cli/lib/config/schema.json",
  "version": 1,
  "projects": {
    "angular-version-migration": {
      "projectType": "application",
      "root": "",
      "sourceRoot": "src",
      "architect": {
        "build": {
          "builder": "@angular-devkit/build-angular:browser",
          "options": {
            "outputPath": "dist",
            "index": "src/index.html",
            "main": "src/main.ts",
            "tsConfig": "tsconfig.app.json",
            "styles": ["src/styles.css"]
          }
        },
        "serve": {
          "builder": "@angular-devkit/build-angular:dev-server",
          "options": {
            "browserTarget": "angular-version-migration:build"
          }
        }
      }
    }
  }
}`,
			"src/main.ts": `import { platformBrowserDynamic } from '@angular/platform-browser-dynamic';
import { AppModule } from './app/app.module';

platformBrowserDynamic().bootstrapModule(AppModule)
  .catch(err => console.error(err));
`,
			"src/app/app.module.ts": `import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';
import { HttpClientModule } from '@angular/common/http';
import { RouterModule, Routes } from '@angular/router';
import { AppComponent } from './app.component';
import { HomeComponent } from './home/home.component';

const routes: Routes = [
  { path: '', component: HomeComponent },
];

@NgModule({
  declarations: [
    AppComponent,
    HomeComponent
  ],
  imports: [
    BrowserModule,
    HttpClientModule,
    RouterModule.forRoot(routes)
  ],
  providers: [],
  bootstrap: [AppComponent]
})
export class AppModule { }
`,
			"src/app/app.component.ts": `import { Component } from '@angular/core';

@Component({
  selector: 'app-root',
  template: '<h1>Angular 15 App (Pre-Migration)</h1><router-outlet></router-outlet>',
  styles: ['h1 { text-align: center; padding: 20px; }']
})
export class AppComponent {
  title = 'Angular 15 App';
}
`,
			"src/app/home/home.component.ts": `import { Component } from '@angular/core';

@Component({
  selector: 'app-home',
  template: '<div class="container"><h2>Home Component</h2><p>This NgModule-based component needs migration to standalone</p></div>',
  styles: ['.container { padding: 20px; }']
})
export class HomeComponent {}
`,
			"src/index.html": `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Angular Migration</title>
  <base href="/">
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <app-root></app-root>
</body>
</html>`,
			"src/styles.css": `body {
  margin: 0;
  font-family: system-ui, -apple-system, sans-serif;
}
`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ES2022",
    "lib": ["ES2022", "dom"],
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  }
}`,
			"tsconfig.app.json": `{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "outDir": "./out-tsc/app",
    "types": []
  },
  "files": [
    "src/main.ts"
  ]
}`,
		},
	}

	// COBOL Modernization
	s.sampleProjects["cobol-modernization"] = &SampleProjectCode{
		ID:           "cobol-modernization",
		Name:         "Legacy COBOL Modernization",
		Description:  "Analyze COBOL code and implement in modern Python",
		Technologies: []string{"COBOL", "Python", "Legacy Modernization"},
		Language:     "cobol",
		StartupScript: `#!/bin/bash
set -euo pipefail

echo "📂 Working in: $(pwd)"

# Add user's local bin to PATH (for current session only)
export PATH="$HOME/.local/bin:$PATH"

# Update package list first
echo "📦 Updating package list..."
sudo apt-get update

# Install Python for modern implementation
if ! command -v python3 &> /dev/null; then
    echo "📦 Installing Python 3..."
    sudo apt-get install -y python3 python3-pip python3-venv
fi

if ! command -v pip3 &> /dev/null; then
    echo "📦 Installing pip3..."
    sudo apt-get install -y python3-pip
fi

# Install GnuCOBOL compiler for running legacy code
if ! command -v cobc &> /dev/null; then
    echo "📦 Installing GnuCOBOL..."
    sudo apt-get install -y gnucobol
    echo "✅ GnuCOBOL installed"
fi

# Install Python dependencies
if [ -f "requirements.txt" ]; then
    echo "📦 Installing Python dependencies..."
    pip3 install --break-system-packages -r requirements.txt
fi

echo ""
echo "✅ Development environment ready"
echo ""
echo "📋 Available Programs:"
echo "  Legacy:  COBOL/batch-processor.cob (COBOL source)"
echo "  Modern:  python/batch_processor.py (Python implementation)"
echo ""
echo "🔧 Commands:"
echo "  Compile COBOL: cobc -x COBOL/batch-processor.cob -o batch-processor"
echo "  Run COBOL:     ./batch-processor"
echo "  Run Python:    python3 python/batch_processor.py"
echo "  Compare:       diff output-cobol.txt output-python.txt"
`,
		GitIgnore: `# COBOL
*.exe
batch-processor

# Python
__pycache__/
*.pyc
.venv/

# Output files
output-*.txt
*.log
`,
		Files: map[string]string{
			"README.md": `# Legacy COBOL Modernization

Modernize COBOL batch processing to Python.

## The Challenge

Legacy COBOL program that processes customer transaction files needs modernization:
- Runs on mainframe (expensive, hard to maintain)
- COBOL developers retiring
- Need modern implementation with identical logic
- Must validate outputs match exactly

## Modernization Workflow

1. **Analyze** - Understand COBOL business logic
2. **Specify** - Write requirements from COBOL behavior
3. **Design** - Create modern Python architecture
4. **Implement** - Code Python version
5. **Validate** - Compare outputs with COBOL
6. **Document** - Create mapping and runbook

## Files

- ` + "`COBOL/batch-processor.cob`" + ` - Legacy COBOL source
- ` + "`COBOL/sample-input.txt`" + ` - Test input file
- ` + "`python/batch_processor.py`" + ` - Modern Python implementation (to be created)
- ` + "`tests/`" + ` - Validation test suite

## Quick Start

` + "```bash" + `
# Compile and run COBOL version
cobc -x COBOL/batch-processor.cob -o batch-processor
./batch-processor

# Run Python version (after implementation)
python3 python/batch_processor.py

# Compare outputs
diff output-cobol.txt output-python.txt
` + "```" + `
`,
			"requirements.txt": `pandas>=2.2.0
pydantic>=2.10.0
`,
			"COBOL/batch-processor.cob": `       IDENTIFICATION DIVISION.
       PROGRAM-ID. BATCH-PROCESSOR.
       AUTHOR. LEGACY-SYSTEMS-TEAM.
      *****************************************************************
      * BATCH TRANSACTION PROCESSOR                                   *
      * Processes customer transactions from input file               *
      * Calculates totals and generates summary report                *
      *****************************************************************

       ENVIRONMENT DIVISION.
       INPUT-OUTPUT SECTION.
       FILE-CONTROL.
           SELECT INPUT-FILE ASSIGN TO "COBOL/sample-input.txt"
               ORGANIZATION IS LINE SEQUENTIAL.
           SELECT OUTPUT-FILE ASSIGN TO "output-cobol.txt"
               ORGANIZATION IS LINE SEQUENTIAL.

       DATA DIVISION.
       FILE SECTION.
       FD  INPUT-FILE.
       01  INPUT-RECORD.
           05 CUST-ID            PIC 9(6).
           05 CUST-NAME          PIC X(30).
           05 TRANS-AMOUNT       PIC 9(7)V99.
           05 TRANS-TYPE         PIC X(1).

       FD  OUTPUT-FILE.
       01  OUTPUT-RECORD         PIC X(80).

       WORKING-STORAGE SECTION.
       01  WS-EOF                PIC X VALUE 'N'.
       01  WS-RECORD-COUNT       PIC 9(6) VALUE 0.
       01  WS-TOTAL-CREDITS      PIC 9(10)V99 VALUE 0.
       01  WS-TOTAL-DEBITS       PIC 9(10)V99 VALUE 0.
       01  WS-NET-AMOUNT         PIC S9(10)V99 VALUE 0.

       01  OUTPUT-LINE.
           05 FILLER            PIC X(30) VALUE SPACES.
           05 OUT-CUST-ID       PIC 9(6).
           05 FILLER            PIC X(3) VALUE SPACES.
           05 OUT-AMOUNT        PIC ZZZ,ZZZ,ZZ9.99.
           05 FILLER            PIC X(3) VALUE SPACES.
           05 OUT-TYPE          PIC X(6).

       PROCEDURE DIVISION.
       MAIN-LOGIC.
           OPEN INPUT INPUT-FILE
           OPEN OUTPUT OUTPUT-FILE

           PERFORM PROCESS-RECORDS UNTIL WS-EOF = 'Y'
           PERFORM WRITE-SUMMARY

           CLOSE INPUT-FILE
           CLOSE OUTPUT-FILE
           STOP RUN.

       PROCESS-RECORDS.
           READ INPUT-FILE
               AT END MOVE 'Y' TO WS-EOF
               NOT AT END PERFORM PROCESS-ONE-RECORD
           END-READ.

       PROCESS-ONE-RECORD.
           ADD 1 TO WS-RECORD-COUNT

           IF TRANS-TYPE = 'C'
               ADD TRANS-AMOUNT TO WS-TOTAL-CREDITS
               MOVE 'CREDIT' TO OUT-TYPE
           ELSE
               ADD TRANS-AMOUNT TO WS-TOTAL-DEBITS
               MOVE 'DEBIT' TO OUT-TYPE
           END-IF

           MOVE CUST-ID TO OUT-CUST-ID
           MOVE TRANS-AMOUNT TO OUT-AMOUNT
           WRITE OUTPUT-RECORD FROM OUTPUT-LINE.

       WRITE-SUMMARY.
           COMPUTE WS-NET-AMOUNT = WS-TOTAL-CREDITS - WS-TOTAL-DEBITS

           MOVE SPACES TO OUTPUT-LINE
           MOVE 'SUMMARY: TOTAL RECORDS PROCESSED: ' TO OUTPUT-LINE
           STRING 'SUMMARY: ' WS-RECORD-COUNT ' RECORDS'
               DELIMITED BY SIZE INTO OUTPUT-LINE
           WRITE OUTPUT-RECORD FROM OUTPUT-LINE

           MOVE SPACES TO OUTPUT-LINE
           STRING 'TOTAL CREDITS: ' WS-TOTAL-CREDITS
               DELIMITED BY SIZE INTO OUTPUT-LINE
           WRITE OUTPUT-RECORD FROM OUTPUT-LINE

           MOVE SPACES TO OUTPUT-LINE
           STRING 'TOTAL DEBITS:  ' WS-TOTAL-DEBITS
               DELIMITED BY SIZE INTO OUTPUT-LINE
           WRITE OUTPUT-RECORD FROM OUTPUT-LINE.
`,
			"COBOL/sample-input.txt": `000001Customer One                  0010050C
000002Customer Two                  0005000D
000003Customer Three                0015075C
000004Customer Four                 0002500D
000005Customer Five                 0030000C
`,
			"COBOL/README.md": `# Legacy COBOL Code

This COBOL program processes customer transactions.

## Compilation

` + "```bash" + `
cobc -x batch-processor.cob -o batch-processor
./batch-processor
` + "```" + `

## Input Format

Fixed-width format:
- Positions 1-6: Customer ID (6 digits)
- Positions 7-36: Customer Name (30 chars)
- Positions 37-45: Transaction Amount (9 digits, 2 decimal places)
- Position 46: Transaction Type (C=Credit, D=Debit)

## Business Logic

1. Read each transaction record
2. Accumulate credits and debits separately
3. Write formatted transaction line to output
4. Generate summary with totals and net amount
`,
			"python/README.md": `# Modern Python Implementation

This directory will contain the Python version of the COBOL batch processor.

## Target Implementation

` + "```python" + `
# batch_processor.py - To be implemented
# Should produce identical output to COBOL version
` + "```" + `

## Validation

After implementation, compare outputs:
` + "```bash" + `
diff output-cobol.txt output-python.txt
` + "```" + `

Outputs should be identical.
`,
		},
	}

	// Clone Demo - Shape Circle (Start Here)
	s.sampleProjects["clone-demo-shape-circle"] = &SampleProjectCode{
		ID:           "clone-demo-shape-circle",
		Name:         "Circle Shape (Start Here)",
		Description:  "Circle shape - START HERE for clone demo",
		Technologies: []string{"SVG", "HTML", "CSS"},
		Language:     "html",
		StartupScript: `#!/bin/bash
set -euo pipefail
echo "🔵 Circle Shape Project"
echo "📂 Working in: $(pwd)"

# Start a simple HTTP server and open the viewer
echo "🌐 Starting shape viewer..."
python3 -m http.server 8080 &
sleep 1

# Open in browser
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:8080/viewer.html
elif command -v open &> /dev/null; then
    open http://localhost:8080/viewer.html
fi

echo "✅ Shape viewer running at http://localhost:8080/viewer.html"
echo ""
echo "📋 This is the START HERE project for the clone demo"
echo "1. The agent will ask you for your brand color"
echo "2. Once the circle is filled, clone the task to the other 4 shapes"
`,
		GitIgnore: `.DS_Store
`,
		Files: map[string]string{
			"README.md": `# Circle Shape (Start Here)

This project contains a circle shape that needs to be filled with your brand color.

## Clone Demo Instructions

**This is the starting point for the clone feature demo.**

1. Start the task - the agent will ask you for your brand color
2. Once the circle is filled with your color, click "Clone" on the task card
3. Select the other 4 shape projects (Square, Triangle, Hexagon, Star)
4. Watch the cloned tasks apply the same color to all shapes

## Files

- ` + "`shape.svg`" + ` - The circle shape (currently white with black outline)
- ` + "`viewer.html`" + ` - Auto-refreshing viewer to see changes live

## The Task

Fill the circle with the company's brand color. The color is NOT specified in the code - you must ask the user.
`,
			"shape.svg": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <circle cx="100" cy="100" r="80" fill="white" stroke="black" stroke-width="3"/>
</svg>
`,
			"viewer.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Circle Shape Viewer</title>
    <meta http-equiv="refresh" content="1">
    <style>
        body {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            color: white;
        }
        h1 { margin-bottom: 2rem; color: #eee; }
        .shape-container {
            background: white;
            padding: 2rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
        }
        img { display: block; width: 200px; height: 200px; }
        .hint { margin-top: 2rem; color: #888; font-size: 0.9rem; }
    </style>
</head>
<body>
    <h1>Circle</h1>
    <div class="shape-container">
        <img src="shape.svg" alt="Circle shape">
    </div>
    <p class="hint">This page refreshes every second to show changes</p>
</body>
</html>
`,
		},
	}

	// Clone Demo - Shape Square
	s.sampleProjects["clone-demo-shape-square"] = &SampleProjectCode{
		ID:           "clone-demo-shape-square",
		Name:         "Square Shape",
		Description:  "Square shape - clone target",
		Technologies: []string{"SVG", "HTML", "CSS"},
		Language:     "html",
		StartupScript: `#!/bin/bash
set -euo pipefail
echo "🟦 Square Shape Project"
python3 -m http.server 8080 &
sleep 1
if command -v xdg-open &> /dev/null; then xdg-open http://localhost:8080/viewer.html; fi
echo "✅ Shape viewer running at http://localhost:8080/viewer.html"
`,
		GitIgnore: `.DS_Store
`,
		Files: map[string]string{
			"README.md": `# Square Shape

This project contains a square shape that needs to be filled with the brand color.

The brand color should already be specified in the task spec from the cloned task.
`,
			"shape.svg": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <rect x="20" y="20" width="160" height="160" fill="white" stroke="black" stroke-width="3"/>
</svg>
`,
			"viewer.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Square Shape Viewer</title>
    <meta http-equiv="refresh" content="1">
    <style>
        body {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            color: white;
        }
        h1 { margin-bottom: 2rem; color: #eee; }
        .shape-container {
            background: white;
            padding: 2rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
        }
        img { display: block; width: 200px; height: 200px; }
        .hint { margin-top: 2rem; color: #888; font-size: 0.9rem; }
    </style>
</head>
<body>
    <h1>Square</h1>
    <div class="shape-container">
        <img src="shape.svg" alt="Square shape">
    </div>
    <p class="hint">This page refreshes every second to show changes</p>
</body>
</html>
`,
		},
	}

	// Clone Demo - Shape Triangle
	s.sampleProjects["clone-demo-shape-triangle"] = &SampleProjectCode{
		ID:           "clone-demo-shape-triangle",
		Name:         "Triangle Shape",
		Description:  "Triangle shape - clone target",
		Technologies: []string{"SVG", "HTML", "CSS"},
		Language:     "html",
		StartupScript: `#!/bin/bash
set -euo pipefail
echo "🔺 Triangle Shape Project"
python3 -m http.server 8080 &
sleep 1
if command -v xdg-open &> /dev/null; then xdg-open http://localhost:8080/viewer.html; fi
echo "✅ Shape viewer running at http://localhost:8080/viewer.html"
`,
		GitIgnore: `.DS_Store
`,
		Files: map[string]string{
			"README.md": `# Triangle Shape

This project contains a triangle shape that needs to be filled with the brand color.

The brand color should already be specified in the task spec from the cloned task.
`,
			"shape.svg": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <polygon points="100,20 180,180 20,180" fill="white" stroke="black" stroke-width="3"/>
</svg>
`,
			"viewer.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Triangle Shape Viewer</title>
    <meta http-equiv="refresh" content="1">
    <style>
        body {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            color: white;
        }
        h1 { margin-bottom: 2rem; color: #eee; }
        .shape-container {
            background: white;
            padding: 2rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
        }
        img { display: block; width: 200px; height: 200px; }
        .hint { margin-top: 2rem; color: #888; font-size: 0.9rem; }
    </style>
</head>
<body>
    <h1>Triangle</h1>
    <div class="shape-container">
        <img src="shape.svg" alt="Triangle shape">
    </div>
    <p class="hint">This page refreshes every second to show changes</p>
</body>
</html>
`,
		},
	}

	// Clone Demo - Shape Hexagon
	s.sampleProjects["clone-demo-shape-hexagon"] = &SampleProjectCode{
		ID:           "clone-demo-shape-hexagon",
		Name:         "Hexagon Shape",
		Description:  "Hexagon shape - clone target",
		Technologies: []string{"SVG", "HTML", "CSS"},
		Language:     "html",
		StartupScript: `#!/bin/bash
set -euo pipefail
echo "⬡ Hexagon Shape Project"
python3 -m http.server 8080 &
sleep 1
if command -v xdg-open &> /dev/null; then xdg-open http://localhost:8080/viewer.html; fi
echo "✅ Shape viewer running at http://localhost:8080/viewer.html"
`,
		GitIgnore: `.DS_Store
`,
		Files: map[string]string{
			"README.md": `# Hexagon Shape

This project contains a hexagon shape that needs to be filled with the brand color.

The brand color should already be specified in the task spec from the cloned task.
`,
			"shape.svg": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <polygon points="100,10 178,55 178,145 100,190 22,145 22,55" fill="white" stroke="black" stroke-width="3"/>
</svg>
`,
			"viewer.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Hexagon Shape Viewer</title>
    <meta http-equiv="refresh" content="1">
    <style>
        body {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            color: white;
        }
        h1 { margin-bottom: 2rem; color: #eee; }
        .shape-container {
            background: white;
            padding: 2rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
        }
        img { display: block; width: 200px; height: 200px; }
        .hint { margin-top: 2rem; color: #888; font-size: 0.9rem; }
    </style>
</head>
<body>
    <h1>Hexagon</h1>
    <div class="shape-container">
        <img src="shape.svg" alt="Hexagon shape">
    </div>
    <p class="hint">This page refreshes every second to show changes</p>
</body>
</html>
`,
		},
	}

	// Clone Demo - Shape Star
	s.sampleProjects["clone-demo-shape-star"] = &SampleProjectCode{
		ID:           "clone-demo-shape-star",
		Name:         "Star Shape",
		Description:  "Star shape - clone target",
		Technologies: []string{"SVG", "HTML", "CSS"},
		Language:     "html",
		StartupScript: `#!/bin/bash
set -euo pipefail
echo "⭐ Star Shape Project"
python3 -m http.server 8080 &
sleep 1
if command -v xdg-open &> /dev/null; then xdg-open http://localhost:8080/viewer.html; fi
echo "✅ Shape viewer running at http://localhost:8080/viewer.html"
`,
		GitIgnore: `.DS_Store
`,
		Files: map[string]string{
			"README.md": `# Star Shape

This project contains a star shape that needs to be filled with the brand color.

The brand color should already be specified in the task spec from the cloned task.
`,
			"shape.svg": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <polygon points="100,10 120,75 190,75 135,115 155,180 100,145 45,180 65,115 10,75 80,75" fill="white" stroke="black" stroke-width="3"/>
</svg>
`,
			"viewer.html": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Star Shape Viewer</title>
    <meta http-equiv="refresh" content="1">
    <style>
        body {
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
            background: #1a1a2e;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            color: white;
        }
        h1 { margin-bottom: 2rem; color: #eee; }
        .shape-container {
            background: white;
            padding: 2rem;
            border-radius: 1rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
        }
        img { display: block; width: 200px; height: 200px; }
        .hint { margin-top: 2rem; color: #888; font-size: 0.9rem; }
    </style>
</head>
<body>
    <h1>Star</h1>
    <div class="shape-container">
        <img src="shape.svg" alt="Star shape">
    </div>
    <p class="hint">This page refreshes every second to show changes</p>
</body>
</html>
`,
		},
	}
}

// GetProjectCodeArchive returns a compressed archive of all project files
func (s *SampleProjectCodeService) GetProjectCodeArchive(ctx context.Context, projectID string) (map[string]string, error) {
	project, err := s.GetProjectCode(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Add .gitignore to files
	allFiles := make(map[string]string)
	for path, content := range project.Files {
		allFiles[path] = content
	}
	allFiles[".gitignore"] = project.GitIgnore

	return allFiles, nil
}
