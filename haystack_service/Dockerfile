FROM ghcr.io/astral-sh/uv:0.10.2@sha256:94a23af2d50e97b87b522d3cea24aaf8a1faedec1344c952767434f69585cbf9 AS uv
FROM ghcr.io/astral-sh/uv:0.10.2-debian-slim@sha256:12d42c65cf6fefafa613cde7bea7a3906d0438364f4af3bd733a398a5e046b9d AS diffusers-build-env
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

# Need to install certifi to avoid SSL errors during the NLTK download
RUN apt-get update && apt-get install -y \
    python3-certifi \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY pyproject.toml uv.lock .python-version ./
RUN --mount=type=cache,id=uv-cache-v2,target=/root/.cache/uv \
    UV_HTTP_TIMEOUT=300 uv sync --frozen --no-dev

# Create directory for NLTK data
RUN mkdir -p /usr/share/nltk_data

# Pre-download NLTK resources for document_splitter.py and unstructured PDF processing
# Specify the download directory and ensure files are accessible
# Use cache mounts for both UV packages and NLTK data
RUN --mount=type=cache,id=uv-cache-v2,target=/root/.cache/uv \
    --mount=type=cache,target=/root/nltk_data \
    UV_HTTP_TIMEOUT=300 uv run python -m nltk.downloader -d /usr/share/nltk_data punkt_tab averaged_perceptron_tagger_eng

# Verify the files were downloaded
RUN ls -la /usr/share/nltk_data/tokenizers/ || echo "Tokenizers directory not found or empty"
RUN ls -la /usr/share/nltk_data/taggers/ || echo "Taggers directory not found or empty"

FROM python:3.11-slim@sha256:233de06753d30d120b1a3ce359d8d3be8bda78524cd8f520c99883bfe33964cf

# Install system dependencies for unstructured
RUN apt-get update && apt-get install -y \
    pandoc \
    poppler-utils \
    tesseract-ocr \
    libmagic1 \
    libpq-dev \
    gcc \
    python3-dev \
    libgl1 \
    libglib2.0-0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Set environment settings
WORKDIR /app
ENV NLTK_DATA=/usr/share/nltk_data
ENV HAYSTACK_TELEMETRY_ENABLED=False

# Copy the diffusers build environment including Python and dependencies
COPY --from=uv /uv /bin/uv
COPY --from=diffusers-build-env /usr/share/nltk_data /usr/share/nltk_data
COPY --from=diffusers-build-env /app/.venv /app/.venv
COPY pyproject.toml uv.lock .python-version ./

# Copy the application code first so sync can find it
COPY . /app

# Sync the project dependencies with the existing virtual environment
# Use cache mount to avoid re-downloading packages (even if venv is recreated)
# Increase timeout to 5 minutes for large ML packages
RUN --mount=type=cache,id=uv-cache-v2,target=/root/.cache/uv \
    UV_HTTP_TIMEOUT=300 uv sync --frozen --no-dev

# Set PATH to use the virtual environment
ENV PATH="/app/.venv/bin:$PATH"

# Expose port
EXPOSE 8000

# Run the service
ENTRYPOINT ["uv", "run", "--frozen", "--no-dev"]
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
