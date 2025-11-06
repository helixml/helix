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

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -exec test -f {}/package.json \; -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository with package.json"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

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
    xdg-open http://localhost:3000 &
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
  server: { port: 3000, host: true }
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

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -exec test -f {}/package.json \; -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository with package.json"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

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

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -exec test -f {}/package.json \; -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository with package.json"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

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
    xdg-open http://localhost:19006 &
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

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -exec test -f {}/package.json \; -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository with package.json"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

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
    xdg-open http://localhost:3000 &
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

# Find the code repository directory (script runs from /home/retro/work)
CODE_DIR=$(find . -mindepth 1 -maxdepth 1 -type d ! -name ".*" -exec test -f {}/package.json \; -print -quit)

if [ -z "$CODE_DIR" ]; then
    echo "❌ Error: Could not find code repository with package.json"
    exit 1
fi

cd "$CODE_DIR"
echo "📂 Working in: $CODE_DIR"

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
    xdg-open http://localhost:3000 &
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
