.PHONY: run build test install clean

# Build and run the TTS server
run:
	@echo "🤖 Starting HyprMoon TTS Server..."
	go run main.go

# Build binary
build:
	@echo "🔨 Building TTS server..."
	go build -o tts-server main.go

# Install dependencies
install:
	@echo "📦 Installing dependencies..."
	go mod tidy
	@echo "🎤 Checking for Coqui TTS engine..."
	@which tts || (echo "❌ Please install Coqui TTS: pip install TTS" && exit 1)
	@echo "✅ All dependencies ready!"

# Test the server
test:
	@echo "🧪 Testing TTS server..."
	@echo "Starting server in background..."
	@go run main.go &
	@SERVER_PID=$$!; \
	sleep 2; \
	echo "Testing basic endpoint..."; \
	curl -f http://localhost:8080/health || (kill $$SERVER_PID; exit 1); \
	echo "Testing TTS synthesis..."; \
	curl -f -X POST http://localhost:8080/speak \
		-H "Content-Type: application/json" \
		-d '{"text": "Test successful"}' || (kill $$SERVER_PID; exit 1); \
	kill $$SERVER_PID; \
	echo "✅ All tests passed!"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f tts-server

# Install system dependencies (Ubuntu/Debian)
install-system:
	@echo "📦 Installing system dependencies..."
	sudo apt update
	sudo apt install -y python3 python3-pip alsa-utils
	pip install TTS
	@echo "✅ System dependencies installed!"

# Quick demo
demo:
	@echo "🎭 Running TTS demo..."
	go run main.go &
	@SERVER_PID=$$!; \
	sleep 2; \
	echo "🎤 Testing robot voice..."; \
	curl -X POST http://localhost:8080/speak \
		-H "Content-Type: application/json" \
		-d '{"text": "Hades has connected to the test chamber", "voice": "p225", "speed": 1.0}'; \
	sleep 3; \
	curl -X POST http://localhost:8080/speak \
		-H "Content-Type: application/json" \
		-d '{"text": "WebRTC stream established", "voice": "p232", "speed": 1.0}'; \
	sleep 3; \
	kill $$SERVER_PID; \
	echo "🎯 Demo complete!"

# Development mode with auto-reload
dev:
	@echo "🔄 Starting development server with auto-reload..."
	@which air > /dev/null || (echo "Installing air..."; go install github.com/cosmtrek/air@latest)
	air

# Production build
prod:
	@echo "🚀 Building production binary..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tts-server main.go
	@echo "✅ Production binary ready: ./tts-server"