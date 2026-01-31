.PHONY: install
install:
	uv sync

.PHONY: test
test: install
	uv run pytest tests

.PHONY: lint
lint: install
	uv run ruff check

.PHONY: build
build: install
	docker build -t registry.helixml.tech/helix/haystack:latest .

.PHONY: docker
docker: build
	docker run -p 8000:8000 registry.helixml.tech/helix/haystack:latest

.PHONY: dev
dev: install
	uv run uvicorn main:app --host 0.0.0.0 --port 8000 --reload
