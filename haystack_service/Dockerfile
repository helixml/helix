ARG UV_VERSION="0.6.9"

FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv
FROM ghcr.io/astral-sh/uv:${UV_VERSION}-bookworm-slim AS diffusers-build-env
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy

# Need to install certifi to avoid SSL errors during the NLTK download
RUN apt-get update && apt-get install -y \
    python3-certifi \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN --mount=type=cache,target=/root/.cache/uv \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=.python-version,target=.python-version \
    uv sync --frozen --no-install-project --no-dev

# Create directory for NLTK data
RUN mkdir -p /usr/share/nltk_data

# Pre-download NLTK resources for document_splitter.py and unstructured PDF processing
# Specify the download directory and ensure files are accessible
RUN uv run python -m nltk.downloader -d /usr/share/nltk_data punkt_tab averaged_perceptron_tagger_eng

# Verify the files were downloaded
RUN ls -la /usr/share/nltk_data/tokenizers/ || echo "Tokenizers directory not found or empty"
RUN ls -la /usr/share/nltk_data/taggers/ || echo "Taggers directory not found or empty"

ADD . /app
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

FROM python:3.11-slim

# Install system dependencies for unstructured
RUN apt-get update && apt-get install -y \
    pandoc \
    poppler-utils \
    tesseract-ocr \
    libmagic1 \
    libpq-dev \
    gcc \
    python3-dev \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*


# Set environment settings
WORKDIR /app
ENV NLTK_DATA=/usr/share/nltk_data
ENV HAYSTACK_TELEMETRY_ENABLED=False

# Expose port
EXPOSE 8000

# Run the service
ENTRYPOINT ["uv", "run", "--frozen", "--no-dev"]
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"] 

# Copy the diffusers build environment including Python
COPY --from=uv /uv /bin/uv
COPY --from=diffusers-build-env /usr/share/nltk_data /usr/share/nltk_data
COPY --from=diffusers-build-env /app /app