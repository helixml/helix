FROM python:3.9-slim

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

# Set up working directory
WORKDIR /app

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Pre-download NLTK resources for document_splitter.py
RUN python -c "import nltk; nltk.download('punkt_tab')"

# Copy application code
COPY . .

# Expose port
EXPOSE 8000

ENV HAYSTACK_TELEMETRY_ENABLED=False

# Run the service
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"] 