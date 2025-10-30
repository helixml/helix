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
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	GitHubRepo   string            `json:"github_repo"`
	Technologies []string          `json:"technologies"`
	Files        map[string]string `json:"files"` // filepath -> content
	GitIgnore    string            `json:"gitignore"`
	ReadmeURL    string            `json:"readme_url"`
	Language     string            `json:"language"`
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
	// React Todo App
	s.sampleProjects["react-todo-app"] = &SampleProjectCode{
		ID:           "react-todo-app",
		Name:         "React Todo Application",
		Description:  "A modern todo application built with React, TypeScript, and Material-UI",
		GitHubRepo:   "sample-react-todo",
		Technologies: []string{"React", "TypeScript", "Material-UI", "Vite"},
		Language:     "javascript",
		ReadmeURL:    "/sample-projects/react-todo-app/README.md",
		GitIgnore: `# Dependencies
node_modules/
.pnpm-debug.log*

# Production build
dist/
build/

# Environment variables
.env.local
.env.development.local
.env.test.local
.env.production.local

# IDE
.vscode/
.idea/

# OS
.DS_Store
Thumbs.db`,
		Files: map[string]string{
			"package.json": `{
  "name": "react-todo-app",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "test": "vitest"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "@mui/material": "^5.14.0",
    "@emotion/react": "^11.11.0",
    "@emotion/styled": "^11.11.0",
    "@mui/icons-material": "^5.14.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^4.4.0",
    "vitest": "^0.34.0"
  }
}`,
			"src/App.tsx": `import React, { useState } from 'react';
import { Container, Typography, Paper, Box } from '@mui/material';
import { TodoList } from './components/TodoList';
import { AddTodo } from './components/AddTodo';
import { Todo } from './types/Todo';

function App() {
  const [todos, setTodos] = useState<Todo[]>([
    { id: 1, text: 'Learn React', completed: false },
    { id: 2, text: 'Build a todo app', completed: false },
  ]);

  const addTodo = (text: string) => {
    const newTodo: Todo = {
      id: Date.now(),
      text,
      completed: false,
    };
    setTodos([...todos, newTodo]);
  };

  const toggleTodo = (id: number) => {
    setTodos(todos.map(todo =>
      todo.id === id ? { ...todo, completed: !todo.completed } : todo
    ));
  };

  const deleteTodo = (id: number) => {
    setTodos(todos.filter(todo => todo.id !== id));
  };

  return (
    <Container maxWidth="md" sx={{ py: 4 }}>
      <Typography variant="h3" component="h1" gutterBottom align="center">
        Todo App
      </Typography>

      <Paper elevation={3} sx={{ p: 3, mb: 3 }}>
        <AddTodo onAdd={addTodo} />
      </Paper>

      <TodoList
        todos={todos}
        onToggle={toggleTodo}
        onDelete={deleteTodo}
      />
    </Container>
  );
}

export default App;`,
			"src/types/Todo.ts": `export interface Todo {
  id: number;
  text: string;
  completed: boolean;
}`,
			"src/components/TodoList.tsx": `import React from 'react';
import { List, Paper, Typography } from '@mui/material';
import { TodoItem } from './TodoItem';
import { Todo } from '../types/Todo';

interface TodoListProps {
  todos: Todo[];
  onToggle: (id: number) => void;
  onDelete: (id: number) => void;
}

export const TodoList: React.FC<TodoListProps> = ({ todos, onToggle, onDelete }) => {
  if (todos.length === 0) {
    return (
      <Paper elevation={2} sx={{ p: 3, textAlign: 'center' }}>
        <Typography color="textSecondary">
          No todos yet. Add one above!
        </Typography>
      </Paper>
    );
  }

  return (
    <Paper elevation={2}>
      <List>
        {todos.map((todo) => (
          <TodoItem
            key={todo.id}
            todo={todo}
            onToggle={onToggle}
            onDelete={onDelete}
          />
        ))}
      </List>
    </Paper>
  );
};`,
			"src/components/TodoItem.tsx": `import React from 'react';
import {
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  IconButton,
  Checkbox,
} from '@mui/material';
import { Delete as DeleteIcon } from '@mui/icons-material';
import { Todo } from '../types/Todo';

interface TodoItemProps {
  todo: Todo;
  onToggle: (id: number) => void;
  onDelete: (id: number) => void;
}

export const TodoItem: React.FC<TodoItemProps> = ({ todo, onToggle, onDelete }) => {
  return (
    <ListItem
      secondaryAction={
        <IconButton edge="end" onClick={() => onDelete(todo.id)}>
          <DeleteIcon />
        </IconButton>
      }
      disablePadding
    >
      <ListItemButton onClick={() => onToggle(todo.id)} dense>
        <ListItemIcon>
          <Checkbox
            edge="start"
            checked={todo.completed}
            tabIndex={-1}
            disableRipple
          />
        </ListItemIcon>
        <ListItemText
          primary={todo.text}
          sx={{
            textDecoration: todo.completed ? 'line-through' : 'none',
            opacity: todo.completed ? 0.7 : 1,
          }}
        />
      </ListItemButton>
    </ListItem>
  );
};`,
			"src/components/AddTodo.tsx": `import React, { useState } from 'react';
import { Box, TextField, Button } from '@mui/material';
import { Add as AddIcon } from '@mui/icons-material';

interface AddTodoProps {
  onAdd: (text: string) => void;
}

export const AddTodo: React.FC<AddTodoProps> = ({ onAdd }) => {
  const [text, setText] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (text.trim()) {
      onAdd(text.trim());
      setText('');
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ display: 'flex', gap: 2 }}>
      <TextField
        fullWidth
        label="Add a new todo"
        value={text}
        onChange={(e) => setText(e.target.value)}
        variant="outlined"
      />
      <Button
        type="submit"
        variant="contained"
        startIcon={<AddIcon />}
        disabled={!text.trim()}
      >
        Add
      </Button>
    </Box>
  );
};`,
			"vite.config.ts": `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    host: true,
  },
})`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}`,
			"src/main.tsx": `import React from 'react'
import ReactDOM from 'react-dom/client'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import App from './App.tsx'

const theme = createTheme({
  palette: {
    mode: 'light',
    primary: {
      main: '#1976d2',
    },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <App />
    </ThemeProvider>
  </React.StrictMode>,
)`,
			"index.html": `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Todo App</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>`,
		},
	}

	// Express API Server
	s.sampleProjects["express-api-server"] = &SampleProjectCode{
		ID:           "express-api-server",
		Name:         "Express API Server",
		Description:  "A RESTful API server built with Express.js, TypeScript, and PostgreSQL",
		GitHubRepo:   "sample-express-api",
		Technologies: []string{"Node.js", "Express", "TypeScript", "PostgreSQL", "Prisma"},
		Language:     "javascript",
		ReadmeURL:    "/sample-projects/express-api-server/README.md",
		GitIgnore: `# Dependencies
node_modules/
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# Production build
dist/
build/

# Environment variables
.env
.env.local
.env.development.local
.env.test.local
.env.production.local

# Database
*.db
*.sqlite

# Logs
logs/
*.log

# IDE
.vscode/
.idea/

# OS
.DS_Store
Thumbs.db`,
		Files: map[string]string{
			"package.json": `{
  "name": "express-api-server",
  "version": "1.0.0",
  "description": "A RESTful API server with Express and TypeScript",
  "main": "dist/index.js",
  "scripts": {
    "dev": "nodemon src/index.ts",
    "build": "tsc",
    "start": "node dist/index.js",
    "test": "jest",
    "db:migrate": "prisma migrate dev",
    "db:generate": "prisma generate"
  },
  "dependencies": {
    "express": "^4.18.0",
    "cors": "^2.8.5",
    "helmet": "^7.0.0",
    "morgan": "^1.10.0",
    "@prisma/client": "^5.0.0",
    "bcrypt": "^5.1.0",
    "jsonwebtoken": "^9.0.0",
    "zod": "^3.22.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "@types/express": "^4.17.0",
    "@types/cors": "^2.8.0",
    "@types/morgan": "^1.9.0",
    "@types/bcrypt": "^5.0.0",
    "@types/jsonwebtoken": "^9.0.0",
    "typescript": "^5.0.0",
    "nodemon": "^3.0.0",
    "ts-node": "^10.9.0",
    "jest": "^29.0.0",
    "prisma": "^5.0.0"
  }
}`,
			"src/index.ts": `import express from 'express';
import cors from 'cors';
import helmet from 'helmet';
import morgan from 'morgan';
import { userRoutes } from './routes/users';
import { postRoutes } from './routes/posts';
import { errorHandler } from './middleware/errorHandler';

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(helmet());
app.use(cors());
app.use(morgan('combined'));
app.use(express.json());

// Routes
app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

app.use('/api/users', userRoutes);
app.use('/api/posts', postRoutes);

// Error handling
app.use(errorHandler);

app.listen(PORT, () => {
  console.log(` + "`" + `Server running on port ${PORT}` + "`" + `);
});`,
			"src/routes/users.ts": `import { Router } from 'express';
import { PrismaClient } from '@prisma/client';
import { z } from 'zod';

const router = Router();
const prisma = new PrismaClient();

const createUserSchema = z.object({
  email: z.string().email(),
  name: z.string().min(1),
});

// GET /api/users
router.get('/', async (req, res, next) => {
  try {
    const users = await prisma.user.findMany({
      select: { id: true, email: true, name: true, createdAt: true },
    });
    res.json(users);
  } catch (error) {
    next(error);
  }
});

// GET /api/users/:id
router.get('/:id', async (req, res, next) => {
  try {
    const { id } = req.params;
    const user = await prisma.user.findUnique({
      where: { id: parseInt(id) },
      select: { id: true, email: true, name: true, createdAt: true },
    });

    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    res.json(user);
  } catch (error) {
    next(error);
  }
});

// POST /api/users
router.post('/', async (req, res, next) => {
  try {
    const validatedData = createUserSchema.parse(req.body);

    const user = await prisma.user.create({
      data: validatedData,
      select: { id: true, email: true, name: true, createdAt: true },
    });

    res.status(201).json(user);
  } catch (error) {
    next(error);
  }
});

// PUT /api/users/:id
router.put('/:id', async (req, res, next) => {
  try {
    const { id } = req.params;
    const validatedData = createUserSchema.partial().parse(req.body);

    const user = await prisma.user.update({
      where: { id: parseInt(id) },
      data: validatedData,
      select: { id: true, email: true, name: true, createdAt: true },
    });

    res.json(user);
  } catch (error) {
    next(error);
  }
});

// DELETE /api/users/:id
router.delete('/:id', async (req, res, next) => {
  try {
    const { id } = req.params;
    await prisma.user.delete({
      where: { id: parseInt(id) },
    });

    res.status(204).send();
  } catch (error) {
    next(error);
  }
});

export { router as userRoutes };`,
			"src/routes/posts.ts": `import { Router } from 'express';
import { PrismaClient } from '@prisma/client';
import { z } from 'zod';

const router = Router();
const prisma = new PrismaClient();

const createPostSchema = z.object({
  title: z.string().min(1),
  content: z.string().min(1),
  authorId: z.number(),
});

// GET /api/posts
router.get('/', async (req, res, next) => {
  try {
    const posts = await prisma.post.findMany({
      include: {
        author: {
          select: { id: true, name: true, email: true },
        },
      },
      orderBy: { createdAt: 'desc' },
    });
    res.json(posts);
  } catch (error) {
    next(error);
  }
});

// GET /api/posts/:id
router.get('/:id', async (req, res, next) => {
  try {
    const { id } = req.params;
    const post = await prisma.post.findUnique({
      where: { id: parseInt(id) },
      include: {
        author: {
          select: { id: true, name: true, email: true },
        },
      },
    });

    if (!post) {
      return res.status(404).json({ error: 'Post not found' });
    }

    res.json(post);
  } catch (error) {
    next(error);
  }
});

// POST /api/posts
router.post('/', async (req, res, next) => {
  try {
    const validatedData = createPostSchema.parse(req.body);

    const post = await prisma.post.create({
      data: validatedData,
      include: {
        author: {
          select: { id: true, name: true, email: true },
        },
      },
    });

    res.status(201).json(post);
  } catch (error) {
    next(error);
  }
});

export { router as postRoutes };`,
			"src/middleware/errorHandler.ts": `import { Request, Response, NextFunction } from 'express';
import { ZodError } from 'zod';
import { PrismaClientKnownRequestError } from '@prisma/client/runtime/library';

export const errorHandler = (
  error: Error,
  req: Request,
  res: Response,
  next: NextFunction
) => {
  console.error('Error:', error);

  if (error instanceof ZodError) {
    return res.status(400).json({
      error: 'Validation error',
      details: error.errors,
    });
  }

  if (error instanceof PrismaClientKnownRequestError) {
    if (error.code === 'P2002') {
      return res.status(409).json({
        error: 'A record with this information already exists',
      });
    }
    if (error.code === 'P2025') {
      return res.status(404).json({
        error: 'Record not found',
      });
    }
  }

  res.status(500).json({
    error: 'Internal server error',
    message: process.env.NODE_ENV === 'production' ? undefined : error.message,
  });
};`,
			"prisma/schema.prisma": `// This is your Prisma schema file,
// learn more about it in the docs: https://pris.ly/d/prisma-schema

generator client {
  provider = "prisma-client-js"
}

datasource db {
  provider = "postgresql"
  url      = env("DATABASE_URL")
}

model User {
  id        Int      @id @default(autoincrement())
  email     String   @unique
  name      String
  posts     Post[]
  createdAt DateTime @default(now())
  updatedAt DateTime @updatedAt

  @@map("users")
}

model Post {
  id        Int      @id @default(autoincrement())
  title     String
  content   String
  published Boolean  @default(false)
  author    User     @relation(fields: [authorId], references: [id])
  authorId  Int
  createdAt DateTime @default(now())
  updatedAt DateTime @updatedAt

  @@map("posts")
}`,
			"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "lib": ["ES2020"],
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}`,
		},
	}

	// Python Flask API
	s.sampleProjects["flask-api"] = &SampleProjectCode{
		ID:           "flask-api",
		Name:         "Flask API Server",
		Description:  "A RESTful API server built with Flask, SQLAlchemy, and PostgreSQL",
		GitHubRepo:   "sample-flask-api",
		Technologies: []string{"Python", "Flask", "SQLAlchemy", "PostgreSQL", "Marshmallow"},
		Language:     "python",
		ReadmeURL:    "/sample-projects/flask-api/README.md",
		GitIgnore: `# Byte-compiled / optimized / DLL files
__pycache__/
*.py[cod]
*$py.class

# Distribution / packaging
.Python
build/
develop-eggs/
dist/
downloads/
eggs/
.eggs/
lib/
lib64/
parts/
sdist/
var/
wheels/

# Environment
.env
.venv
env/
venv/
ENV/
env.bak/
venv.bak/

# IDE
.vscode/
.idea/
*.swp
*.swo

# Database
*.db
*.sqlite

# Logs
*.log

# OS
.DS_Store
Thumbs.db`,
		Files: map[string]string{
			"requirements.txt": `Flask==2.3.3
Flask-SQLAlchemy==3.0.5
Flask-Migrate==4.0.5
Flask-CORS==4.0.0
Flask-Marshmallow==0.15.0
marshmallow-sqlalchemy==0.29.0
python-dotenv==1.0.0
psycopg2-binary==2.9.7
gunicorn==21.2.0`,
			"app.py": `from flask import Flask
from flask_cors import CORS
from config import Config
from extensions import db, migrate, ma
from routes import api_bp


def create_app(config_class=Config):
    app = Flask(__name__)
    app.config.from_object(config_class)

    # Initialize extensions
    db.init_app(app)
    migrate.init_app(app, db)
    ma.init_app(app)
    CORS(app)

    # Register blueprints
    app.register_blueprint(api_bp, url_prefix='/api')

    # Health check endpoint
    @app.route('/health')
    def health():
        return {'status': 'ok', 'message': 'Flask API is running'}

    return app


if __name__ == '__main__':
    app = create_app()
    app.run(debug=True, host='0.0.0.0', port=5000)`,
			"config.py": `import os
from dotenv import load_dotenv

load_dotenv()


class Config:
    SECRET_KEY = os.environ.get('SECRET_KEY') or 'dev-secret-key'
    SQLALCHEMY_DATABASE_URI = os.environ.get('DATABASE_URL') or 'postgresql://user:password@localhost/flask_api_db'
    SQLALCHEMY_TRACK_MODIFICATIONS = False`,
			"extensions.py": `from flask_sqlalchemy import SQLAlchemy
from flask_migrate import Migrate
from flask_marshmallow import Marshmallow

db = SQLAlchemy()
migrate = Migrate()
ma = Marshmallow()`,
			"models.py": `from datetime import datetime
from extensions import db


class User(db.Model):
    __tablename__ = 'users'

    id = db.Column(db.Integer, primary_key=True)
    email = db.Column(db.String(120), unique=True, nullable=False)
    name = db.Column(db.String(100), nullable=False)
    created_at = db.Column(db.DateTime, default=datetime.utcnow)
    updated_at = db.Column(db.DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationship
    posts = db.relationship('Post', backref='author', lazy=True, cascade='all, delete-orphan')

    def __repr__(self):
        return f'<User {self.email}>'


class Post(db.Model):
    __tablename__ = 'posts'

    id = db.Column(db.Integer, primary_key=True)
    title = db.Column(db.String(200), nullable=False)
    content = db.Column(db.Text, nullable=False)
    published = db.Column(db.Boolean, default=False)
    author_id = db.Column(db.Integer, db.ForeignKey('users.id'), nullable=False)
    created_at = db.Column(db.DateTime, default=datetime.utcnow)
    updated_at = db.Column(db.DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    def __repr__(self):
        return f'<Post {self.title}>'`,
			"schemas.py": `from marshmallow import fields, validate
from extensions import ma
from models import User, Post


class UserSchema(ma.SQLAlchemyAutoSchema):
    class Meta:
        model = User
        load_instance = True
        exclude = ('updated_at',)

    email = fields.Email(required=True)
    name = fields.Str(required=True, validate=validate.Length(min=1, max=100))


class PostSchema(ma.SQLAlchemyAutoSchema):
    class Meta:
        model = Post
        load_instance = True
        exclude = ('updated_at',)

    title = fields.Str(required=True, validate=validate.Length(min=1, max=200))
    content = fields.Str(required=True, validate=validate.Length(min=1))
    author_id = fields.Int(required=True)
    author = fields.Nested(UserSchema, dump_only=True)


# Schema instances
user_schema = UserSchema()
users_schema = UserSchema(many=True)
post_schema = PostSchema()
posts_schema = PostSchema(many=True)`,
			"routes.py": `from flask import Blueprint, request, jsonify
from marshmallow import ValidationError
from extensions import db
from models import User, Post
from schemas import user_schema, users_schema, post_schema, posts_schema

api_bp = Blueprint('api', __name__)


# User routes
@api_bp.route('/users', methods=['GET'])
def get_users():
    users = User.query.all()
    return users_schema.dump(users)


@api_bp.route('/users/<int:user_id>', methods=['GET'])
def get_user(user_id):
    user = User.query.get_or_404(user_id)
    return user_schema.dump(user)


@api_bp.route('/users', methods=['POST'])
def create_user():
    try:
        user_data = user_schema.load(request.json)
    except ValidationError as err:
        return {'error': 'Validation error', 'details': err.messages}, 400

    # Check if email already exists
    if User.query.filter_by(email=user_data.email).first():
        return {'error': 'Email already exists'}, 409

    db.session.add(user_data)
    db.session.commit()

    return user_schema.dump(user_data), 201


@api_bp.route('/users/<int:user_id>', methods=['PUT'])
def update_user(user_id):
    user = User.query.get_or_404(user_id)

    try:
        user_data = user_schema.load(request.json, partial=True)
    except ValidationError as err:
        return {'error': 'Validation error', 'details': err.messages}, 400

    # Update fields
    if hasattr(user_data, 'email'):
        user.email = user_data.email
    if hasattr(user_data, 'name'):
        user.name = user_data.name

    db.session.commit()
    return user_schema.dump(user)


@api_bp.route('/users/<int:user_id>', methods=['DELETE'])
def delete_user(user_id):
    user = User.query.get_or_404(user_id)
    db.session.delete(user)
    db.session.commit()
    return '', 204


# Post routes
@api_bp.route('/posts', methods=['GET'])
def get_posts():
    posts = Post.query.order_by(Post.created_at.desc()).all()
    return posts_schema.dump(posts)


@api_bp.route('/posts/<int:post_id>', methods=['GET'])
def get_post(post_id):
    post = Post.query.get_or_404(post_id)
    return post_schema.dump(post)


@api_bp.route('/posts', methods=['POST'])
def create_post():
    try:
        post_data = post_schema.load(request.json)
    except ValidationError as err:
        return {'error': 'Validation error', 'details': err.messages}, 400

    # Verify author exists
    if not User.query.get(post_data.author_id):
        return {'error': 'Author not found'}, 404

    db.session.add(post_data)
    db.session.commit()

    return post_schema.dump(post_data), 201`,
		},
	}

	// Add more sample projects as needed
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
