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

# Create directory for NLTK data
RUN mkdir -p /usr/share/nltk_data

# Pre-download NLTK resources for document_splitter.py and unstructured PDF processing
# Specify the download directory and ensure files are accessible
RUN python -m nltk.downloader -d /usr/share/nltk_data punkt_tab averaged_perceptron_tagger_eng

# Verify the files were downloaded
RUN ls -la /usr/share/nltk_data/tokenizers/ || echo "Tokenizers directory not found or empty"
RUN ls -la /usr/share/nltk_data/taggers/ || echo "Taggers directory not found or empty"

# Set environment variable so NLTK knows where to find the data
ENV NLTK_DATA=/usr/share/nltk_data

# Copy application code
COPY . .

# Expose port
EXPOSE 8000

ENV HAYSTACK_TELEMETRY_ENABLED=False

# Run the service
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"] 