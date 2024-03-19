FROM ubuntu:22.04
RUN apt-get update
RUN apt-get install -y \
  python3 \
  python3-pip \
  python3-dev \
  ffmpeg \
  libsm6 \
  libxext6 \
  libmagic-dev \
  poppler-utils \
  tesseract-ocr \
  libreoffice \
  pandoc \
  libcairo2-dev \
  libjpeg-dev \
  libgif-dev \
  libgirepository1.0-dev \
  libdbus-1-dev
WORKDIR /home/app
ADD requirements.txt .
RUN pip install -r requirements.txt
ADD src ./src
ENTRYPOINT ["python3"]
CMD ["src/main.py"]
