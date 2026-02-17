/**
 * MessageProcessor Benchmarks
 *
 * Run with: npx vitest bench --run
 *
 * These benchmarks measure the performance of MessageProcessor to identify
 * bottlenecks and track optimization progress.
 *
 * Target: MessageProcessor.process() < 2ms for typical content
 */

import { bench, describe, beforeAll } from "vitest";
import { MessageProcessor } from "./Markdown";
import {
  TypesSession,
  TypesSessionMetadata,
  TypesSessionMode,
  TypesSessionType,
  TypesOwnerType,
} from "../../api/api";

// ============================================================================
// Test Fixtures
// ============================================================================

// Mock session configuration
const createMockSession = (
  documentIds: Record<string, string> = {},
): TypesSession => ({
  id: "bench-session",
  name: "Benchmark Session",
  created: new Date().toISOString(),
  updated: new Date().toISOString(),
  parent_session: "",
  parent_app: "",
  mode: TypesSessionMode.SessionModeInference,
  type: TypesSessionType.SessionTypeText,
  model_name: "gpt-4",
  lora_dir: "",
  owner: "test-owner",
  owner_type: TypesOwnerType.OwnerTypeUser,
  config: {
    document_ids: documentIds,
  } as TypesSessionMetadata,
  interactions: [],
});

const mockGetFileURL = (filename: string) =>
  `http://localhost:8080/files/${filename}`;

// Small content (~1KB) - typical short response
const SMALL_CONTENT = `
Here's a quick explanation of how to use React hooks:

\`\`\`typescript
import { useState, useEffect } from 'react';

function Counter() {
  const [count, setCount] = useState(0);

  useEffect(() => {
    document.title = \`Count: \${count}\`;
  }, [count]);

  return (
    <button onClick={() => setCount(c => c + 1)}>
      Count: {count}
    </button>
  );
}
\`\`\`

The \`useState\` hook manages local state, while \`useEffect\` handles side effects.
`;

// Medium content (~10KB) - typical detailed response with multiple code blocks
const MEDIUM_CONTENT = `
# Building a Full-Stack Application with React and Node.js

This guide will walk you through creating a complete web application.

## Prerequisites

Before we begin, make sure you have:
- Node.js 18+ installed
- npm or yarn package manager
- Basic knowledge of JavaScript/TypeScript

## Project Setup

First, let's create our project structure:

\`\`\`bash
mkdir my-fullstack-app
cd my-fullstack-app
npm init -y
\`\`\`

## Backend Setup

Create the server configuration:

\`\`\`typescript
// server/index.ts
import express from 'express';
import cors from 'cors';
import { json } from 'body-parser';

const app = express();
const PORT = process.env.PORT || 3001;

app.use(cors());
app.use(json());

// API routes
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

app.get('/api/users', async (req, res) => {
  try {
    const users = await db.query('SELECT * FROM users');
    res.json(users);
  } catch (error) {
    res.status(500).json({ error: 'Failed to fetch users' });
  }
});

app.post('/api/users', async (req, res) => {
  const { name, email } = req.body;

  if (!name || !email) {
    return res.status(400).json({ error: 'Name and email are required' });
  }

  try {
    const user = await db.query(
      'INSERT INTO users (name, email) VALUES ($1, $2) RETURNING *',
      [name, email]
    );
    res.status(201).json(user);
  } catch (error) {
    res.status(500).json({ error: 'Failed to create user' });
  }
});

app.listen(PORT, () => {
  console.log(\`Server running on port \${PORT}\`);
});
\`\`\`

## Frontend Setup

Now let's set up the React frontend:

\`\`\`typescript
// src/App.tsx
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { UserList } from './components/UserList';
import { UserDetail } from './components/UserDetail';
import { CreateUser } from './components/CreateUser';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 minutes
      retry: 3,
    },
  },
});

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<UserList />} />
          <Route path="/users/:id" element={<UserDetail />} />
          <Route path="/users/new" element={<CreateUser />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
\`\`\`

## API Client

Create a type-safe API client:

\`\`\`typescript
// src/api/client.ts
import axios from 'axios';

const api = axios.create({
  baseURL: process.env.REACT_APP_API_URL || 'http://localhost:3001/api',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor for auth
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('auth_token');
  if (token) {
    config.headers.Authorization = \`Bearer \${token}\`;
  }
  return config;
});

// Response interceptor for error handling
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('auth_token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export interface User {
  id: string;
  name: string;
  email: string;
  createdAt: string;
}

export const userApi = {
  getAll: () => api.get<User[]>('/users').then(res => res.data),
  getById: (id: string) => api.get<User>(\`/users/\${id}\`).then(res => res.data),
  create: (data: Omit<User, 'id' | 'createdAt'>) =>
    api.post<User>('/users', data).then(res => res.data),
  update: (id: string, data: Partial<User>) =>
    api.patch<User>(\`/users/\${id}\`, data).then(res => res.data),
  delete: (id: string) => api.delete(\`/users/\${id}\`),
};
\`\`\`

## Components

Here's the UserList component:

\`\`\`typescript
// src/components/UserList.tsx
import { useQuery } from '@tanstack/react-query';
import { userApi } from '../api/client';
import { Link } from 'react-router-dom';

export function UserList() {
  const { data: users, isLoading, error } = useQuery({
    queryKey: ['users'],
    queryFn: userApi.getAll,
  });

  if (isLoading) return <div>Loading users...</div>;
  if (error) return <div>Error loading users</div>;

  return (
    <div className="user-list">
      <h1>Users</h1>
      <Link to="/users/new" className="btn-primary">
        Create New User
      </Link>
      <ul>
        {users?.map(user => (
          <li key={user.id}>
            <Link to={\`/users/\${user.id}\`}>
              {user.name} ({user.email})
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
\`\`\`

## Styling

Add some basic styles:

\`\`\`css
/* src/styles/main.css */
:root {
  --primary-color: #3b82f6;
  --secondary-color: #64748b;
  --background-color: #f8fafc;
  --text-color: #1e293b;
  --border-radius: 8px;
}

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
    Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif;
  background-color: var(--background-color);
  color: var(--text-color);
  line-height: 1.6;
}

.user-list {
  max-width: 800px;
  margin: 2rem auto;
  padding: 0 1rem;
}

.btn-primary {
  display: inline-block;
  padding: 0.75rem 1.5rem;
  background-color: var(--primary-color);
  color: white;
  text-decoration: none;
  border-radius: var(--border-radius);
  transition: background-color 0.2s;
}

.btn-primary:hover {
  background-color: #2563eb;
}
\`\`\`

## Testing

Write tests for your components:

\`\`\`typescript
// src/components/UserList.test.tsx
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { UserList } from './UserList';
import { userApi } from '../api/client';

vi.mock('../api/client');

const mockUsers = [
  { id: '1', name: 'John Doe', email: 'john@example.com', createdAt: '2024-01-01' },
  { id: '2', name: 'Jane Smith', email: 'jane@example.com', createdAt: '2024-01-02' },
];

describe('UserList', () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>{children}</BrowserRouter>
    </QueryClientProvider>
  );

  it('renders users list', async () => {
    vi.mocked(userApi.getAll).mockResolvedValue(mockUsers);

    render(<UserList />, { wrapper });

    await waitFor(() => {
      expect(screen.getByText('John Doe (john@example.com)')).toBeInTheDocument();
      expect(screen.getByText('Jane Smith (jane@example.com)')).toBeInTheDocument();
    });
  });
});
\`\`\`

## Summary

This guide covered:
1. Setting up a Node.js/Express backend
2. Creating a React frontend with React Query
3. Building a type-safe API client
4. Writing component tests

For more advanced topics, check out:
- Authentication with JWT
- Database migrations with Prisma
- Deployment to AWS/GCP
`;

// Large content (~50KB) - extensive documentation with many code blocks
const generateLargeContent = (): string => {
  const sections: string[] = [MEDIUM_CONTENT];

  // Add more sections to reach ~50KB
  for (let i = 0; i < 4; i++) {
    sections.push(`
## Additional Section ${i + 1}

Here's another complex example with multiple patterns:

\`\`\`typescript
// Example ${i + 1}: Advanced patterns
interface Repository<T> {
  findById(id: string): Promise<T | null>;
  findAll(filters?: Partial<T>): Promise<T[]>;
  create(data: Omit<T, 'id'>): Promise<T>;
  update(id: string, data: Partial<T>): Promise<T>;
  delete(id: string): Promise<void>;
}

class BaseRepository<T extends { id: string }> implements Repository<T> {
  constructor(private readonly tableName: string) {}

  async findById(id: string): Promise<T | null> {
    const result = await db.query(
      \`SELECT * FROM \${this.tableName} WHERE id = $1\`,
      [id]
    );
    return result.rows[0] || null;
  }

  async findAll(filters?: Partial<T>): Promise<T[]> {
    let query = \`SELECT * FROM \${this.tableName}\`;
    const values: any[] = [];

    if (filters && Object.keys(filters).length > 0) {
      const conditions = Object.entries(filters)
        .map(([key, _], index) => \`\${key} = $\${index + 1}\`);
      query += \` WHERE \${conditions.join(' AND ')}\`;
      values.push(...Object.values(filters));
    }

    const result = await db.query(query, values);
    return result.rows;
  }

  async create(data: Omit<T, 'id'>): Promise<T> {
    const keys = Object.keys(data);
    const values = Object.values(data);
    const placeholders = keys.map((_, i) => \`$\${i + 1}\`);

    const result = await db.query(
      \`INSERT INTO \${this.tableName} (\${keys.join(', ')})
       VALUES (\${placeholders.join(', ')}) RETURNING *\`,
      values
    );
    return result.rows[0];
  }

  async update(id: string, data: Partial<T>): Promise<T> {
    const keys = Object.keys(data);
    const values = Object.values(data);
    const setClause = keys.map((key, i) => \`\${key} = $\${i + 1}\`);

    const result = await db.query(
      \`UPDATE \${this.tableName} SET \${setClause.join(', ')}
       WHERE id = $\${keys.length + 1} RETURNING *\`,
      [...values, id]
    );
    return result.rows[0];
  }

  async delete(id: string): Promise<void> {
    await db.query(\`DELETE FROM \${this.tableName} WHERE id = $1\`, [id]);
  }
}

// Usage
const userRepository = new BaseRepository<User>('users');
const user = await userRepository.findById('123');
\`\`\`

### Additional Patterns

\`\`\`typescript
// Event sourcing pattern
interface DomainEvent {
  type: string;
  aggregateId: string;
  timestamp: Date;
  payload: unknown;
}

class EventStore {
  private events: DomainEvent[] = [];

  append(event: DomainEvent): void {
    this.events.push(event);
  }

  getEvents(aggregateId: string): DomainEvent[] {
    return this.events.filter(e => e.aggregateId === aggregateId);
  }
}
\`\`\`

| Feature | Status | Notes |
|---------|--------|-------|
| Authentication | ✅ Complete | JWT-based |
| Authorization | ✅ Complete | RBAC |
| Rate Limiting | 🔄 In Progress | Redis-based |
| Caching | 📋 Planned | Redis/Memcached |
`);
  }

  return sections.join("\n");
};

// Content with thinking tags (tests processThinkingTags)
const CONTENT_WITH_THINKING = `
Let me think about this problem.

<think>
First, I need to analyze the requirements:
1. The system needs to handle high throughput
2. Data consistency is critical
3. Latency should be under 100ms

Let me consider different approaches:
- Option A: Use a message queue for async processing
- Option B: Implement CQRS pattern
- Option C: Use event sourcing

I think Option A combined with some elements of Option B would work best.
</think>

Based on my analysis, here's my recommendation:

The best approach would be to use a message queue (like RabbitMQ or Kafka) for asynchronous processing of write operations, while keeping read operations direct to the database for low latency.

\`\`\`typescript
// Message queue handler
class OrderProcessor {
  async processOrder(order: Order): Promise<void> {
    await this.messageQueue.publish('orders', {
      type: 'ORDER_CREATED',
      payload: order,
    });
  }
}
\`\`\`
`;

// Content with XML citations (tests processXmlCitations)
const CONTENT_WITH_CITATIONS = `
According to the documentation, the API uses REST principles.

<excerpts>
<excerpt document_id="api-docs.md">
The API follows RESTful conventions with the following endpoints:
- GET /api/v1/users - List all users
- POST /api/v1/users - Create a new user
- GET /api/v1/users/:id - Get user by ID
</excerpt>
<excerpt document_id="auth-guide.md">
Authentication is handled via JWT tokens passed in the Authorization header.
Tokens expire after 24 hours and must be refreshed using the /auth/refresh endpoint.
</excerpt>
</excerpts>

As shown in the excerpts above, you'll need to authenticate before making API calls.
`;

// Content with document IDs (tests processDocumentIds and processDocumentGroupIds)
const CONTENT_WITH_DOC_IDS = `
I found relevant information in the following documents:

Based on [doc:project-requirements.pdf], the system must support:
- 10,000 concurrent users
- Sub-second response times
- 99.9% uptime SLA

The technical specifications in [doc:architecture.md] outline the recommended tech stack.

For group context, see [docgroup:backend-specs] which contains all backend documentation.
`;

// Content with filter mentions (tests processFilterMentions)
const CONTENT_WITH_FILTER_MENTIONS = `
Looking at the files you mentioned:

The configuration in [filter:config.yaml id=config-123] shows the database settings.

The model definition in [filter:User.ts id=user-model] defines the user schema:

\`\`\`typescript
interface User {
  id: string;
  name: string;
  email: string;
}
\`\`\`
`;

// Huge content (~200KB) with many code blocks - stress test
const generateHugeContent = (): string => {
  const sections: string[] = [];

  // Generate 20 sections, each with multiple code blocks
  for (let i = 0; i < 20; i++) {
    sections.push(`
## Module ${i + 1}: ${["Authentication", "Authorization", "Database", "Caching", "Logging", "Metrics", "Testing", "Deployment", "Monitoring", "Security"][i % 10]}

This module handles critical functionality for the application.

### Overview

The ${["auth", "access", "data", "cache", "log", "metric", "test", "deploy", "monitor", "security"][i % 10]} module provides:
- Feature 1: Important functionality
- Feature 2: Critical operations
- Feature 3: Performance optimizations

### Implementation

\`\`\`typescript
// Module ${i + 1} implementation
import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { Logger } from './logger';

interface Module${i + 1}Config {
  enabled: boolean;
  timeout: number;
  retries: number;
  endpoints: string[];
}

@Injectable()
export class Module${i + 1}Service {
  private readonly logger = new Logger(Module${i + 1}Service.name);
  private readonly config: Module${i + 1}Config;

  constructor(private readonly configService: ConfigService) {
    this.config = {
      enabled: configService.get('MODULE_${i + 1}_ENABLED', true),
      timeout: configService.get('MODULE_${i + 1}_TIMEOUT', 5000),
      retries: configService.get('MODULE_${i + 1}_RETRIES', 3),
      endpoints: configService.get('MODULE_${i + 1}_ENDPOINTS', []),
    };
  }

  async initialize(): Promise<void> {
    this.logger.log('Initializing module ${i + 1}...');

    if (!this.config.enabled) {
      this.logger.warn('Module ${i + 1} is disabled');
      return;
    }

    for (const endpoint of this.config.endpoints) {
      await this.validateEndpoint(endpoint);
    }

    this.logger.log('Module ${i + 1} initialized successfully');
  }

  private async validateEndpoint(endpoint: string): Promise<boolean> {
    try {
      const response = await fetch(endpoint, {
        method: 'HEAD',
        signal: AbortSignal.timeout(this.config.timeout),
      });
      return response.ok;
    } catch (error) {
      this.logger.error(\`Failed to validate endpoint: \${endpoint}\`, error);
      return false;
    }
  }

  async process(data: unknown): Promise<unknown> {
    let lastError: Error | undefined;

    for (let attempt = 1; attempt <= this.config.retries; attempt++) {
      try {
        return await this.doProcess(data);
      } catch (error) {
        lastError = error as Error;
        this.logger.warn(\`Attempt \${attempt} failed: \${lastError.message}\`);

        if (attempt < this.config.retries) {
          await this.delay(Math.pow(2, attempt) * 100);
        }
      }
    }

    throw lastError;
  }

  private async doProcess(data: unknown): Promise<unknown> {
    // Actual processing logic
    return { processed: true, data, timestamp: new Date().toISOString() };
  }

  private delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
\`\`\`

### Configuration

\`\`\`yaml
# config/module-${i + 1}.yaml
module_${i + 1}:
  enabled: true
  timeout: 5000
  retries: 3
  endpoints:
    - https://api.example.com/v1
    - https://backup.example.com/v1
  logging:
    level: info
    format: json
  metrics:
    enabled: true
    interval: 60
\`\`\`

### Testing

\`\`\`typescript
// module-${i + 1}.spec.ts
import { Test, TestingModule } from '@nestjs/testing';
import { ConfigService } from '@nestjs/config';
import { Module${i + 1}Service } from './module-${i + 1}.service';

describe('Module${i + 1}Service', () => {
  let service: Module${i + 1}Service;
  let configService: ConfigService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        Module${i + 1}Service,
        {
          provide: ConfigService,
          useValue: {
            get: jest.fn((key: string, defaultValue: unknown) => defaultValue),
          },
        },
      ],
    }).compile();

    service = module.get<Module${i + 1}Service>(Module${i + 1}Service);
    configService = module.get<ConfigService>(ConfigService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  describe('initialize', () => {
    it('should initialize successfully when enabled', async () => {
      await expect(service.initialize()).resolves.not.toThrow();
    });
  });

  describe('process', () => {
    it('should process data successfully', async () => {
      const data = { test: true };
      const result = await service.process(data);

      expect(result).toHaveProperty('processed', true);
      expect(result).toHaveProperty('data', data);
    });
  });
});
\`\`\`

`);
  }

  return sections.join("\n");
};

// ============================================================================
// Benchmarks
// ============================================================================

let LARGE_CONTENT: string;
let HUGE_CONTENT: string;

beforeAll(() => {
  // Generate large content once before benchmarks
  LARGE_CONTENT = generateLargeContent();
  HUGE_CONTENT = generateHugeContent();
});

describe("MessageProcessor.process() - Full Pipeline", () => {
  const session = createMockSession({
    "api-docs.md": "api-docs.md",
    "auth-guide.md": "auth-guide.md",
    "project-requirements.pdf": "project-requirements.pdf",
    "architecture.md": "architecture.md",
    "config.yaml": "config.yaml",
    "User.ts": "User.ts",
  });

  bench("small content (~1KB)", () => {
    const processor = new MessageProcessor(SMALL_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("small content (~1KB) - streaming", () => {
    const processor = new MessageProcessor(SMALL_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: true,
      showBlinker: true,
    });
    processor.process();
  });

  bench("medium content (~10KB)", () => {
    const processor = new MessageProcessor(MEDIUM_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("medium content (~10KB) - streaming", () => {
    const processor = new MessageProcessor(MEDIUM_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: true,
      showBlinker: true,
    });
    processor.process();
  });

  bench("large content (~50KB)", () => {
    const processor = new MessageProcessor(LARGE_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("large content (~50KB) - streaming", () => {
    const processor = new MessageProcessor(LARGE_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: true,
      showBlinker: true,
    });
    processor.process();
  });

  bench("huge content (~200KB)", () => {
    const processor = new MessageProcessor(HUGE_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("huge content (~200KB) - streaming", () => {
    const processor = new MessageProcessor(HUGE_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: true,
      showBlinker: true,
    });
    processor.process();
  });
});

describe("MessageProcessor - Feature-Specific Benchmarks", () => {
  const session = createMockSession({
    "api-docs.md": "api-docs.md",
    "auth-guide.md": "auth-guide.md",
    "project-requirements.pdf": "project-requirements.pdf",
    "architecture.md": "architecture.md",
    "config.yaml": "config.yaml",
    "User.ts": "User.ts",
  });

  bench("content with thinking tags", () => {
    const processor = new MessageProcessor(CONTENT_WITH_THINKING, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("content with XML citations", () => {
    const processor = new MessageProcessor(CONTENT_WITH_CITATIONS, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("content with document IDs", () => {
    const processor = new MessageProcessor(CONTENT_WITH_DOC_IDS, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("content with filter mentions", () => {
    const processor = new MessageProcessor(CONTENT_WITH_FILTER_MENTIONS, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  // Combine all special content
  const COMBINED_SPECIAL_CONTENT = [
    CONTENT_WITH_THINKING,
    CONTENT_WITH_CITATIONS,
    CONTENT_WITH_DOC_IDS,
    CONTENT_WITH_FILTER_MENTIONS,
  ].join("\n\n");

  bench("combined special content (all features)", () => {
    const processor = new MessageProcessor(COMBINED_SPECIAL_CONTENT, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });
});

describe("MessageProcessor - Streaming Simulation", () => {
  const session = createMockSession();

  // Simulate incremental streaming by processing progressively longer content
  bench("streaming simulation - 10 incremental updates", () => {
    const fullContent = MEDIUM_CONTENT;
    const chunkSize = Math.ceil(fullContent.length / 10);

    for (let i = 1; i <= 10; i++) {
      const partialContent = fullContent.slice(0, i * chunkSize);
      const processor = new MessageProcessor(partialContent, {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: i < 10,
        showBlinker: i < 10,
      });
      processor.process();
    }
  });

  bench("streaming simulation - 50 incremental updates", () => {
    const fullContent = MEDIUM_CONTENT;
    const chunkSize = Math.ceil(fullContent.length / 50);

    for (let i = 1; i <= 50; i++) {
      const partialContent = fullContent.slice(0, i * chunkSize);
      const processor = new MessageProcessor(partialContent, {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: i < 50,
        showBlinker: i < 50,
      });
      processor.process();
    }
  });
});

describe("MessageProcessor - Edge Cases", () => {
  const session = createMockSession();

  bench("empty content", () => {
    const processor = new MessageProcessor("", {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("whitespace only", () => {
    const processor = new MessageProcessor("   \n\n\t\t   \n   ", {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("single code block", () => {
    const processor = new MessageProcessor("```typescript\nconst x = 1;\n```", {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("many small code blocks (20 blocks)", () => {
    const content = Array.from(
      { length: 20 },
      (_, i) => `\`\`\`typescript\nconst var${i} = ${i};\n\`\`\``,
    ).join("\n\n");

    const processor = new MessageProcessor(content, {
      session,
      getFileURL: mockGetFileURL,
      isStreaming: false,
    });
    processor.process();
  });

  bench("unclosed code block (streaming)", () => {
    const processor = new MessageProcessor(
      'Here is some code:\n\n```typescript\nfunction test() {\n  console.log("hello',
      {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: true,
        showBlinker: true,
      },
    );
    processor.process();
  });

  bench("unclosed thinking tag (streaming)", () => {
    const processor = new MessageProcessor(
      "Let me think about this.\n\n<think>\nAnalyzing the problem...",
      {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: true,
        showBlinker: true,
      },
    );
    processor.process();
  });

  bench("partial XML citation (streaming)", () => {
    const processor = new MessageProcessor(
      'Based on the documentation:\n\n<excerpts>\n<excerpt document_id="test.md">\nThis is partial content',
      {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: true,
        showBlinker: true,
      },
    );
    processor.process();
  });

  bench("content ending with triple dash (streaming)", () => {
    const processor = new MessageProcessor(
      "Here is some content that ends with ---",
      {
        session,
        getFileURL: mockGetFileURL,
        isStreaming: true,
        showBlinker: true,
      },
    );
    processor.process();
  });
});

// Report content sizes for reference
describe("Content Size Reference", () => {
  bench("report sizes (informational)", () => {
    console.log("\n=== Content Sizes ===");
    console.log(
      `SMALL_CONTENT: ${SMALL_CONTENT.length} bytes (~${(SMALL_CONTENT.length / 1024).toFixed(1)} KB)`,
    );
    console.log(
      `MEDIUM_CONTENT: ${MEDIUM_CONTENT.length} bytes (~${(MEDIUM_CONTENT.length / 1024).toFixed(1)} KB)`,
    );
    console.log(
      `LARGE_CONTENT: ${LARGE_CONTENT.length} bytes (~${(LARGE_CONTENT.length / 1024).toFixed(1)} KB)`,
    );
    console.log(
      `HUGE_CONTENT: ${HUGE_CONTENT.length} bytes (~${(HUGE_CONTENT.length / 1024).toFixed(1)} KB)`,
    );
    console.log("===================\n");
  });
});
