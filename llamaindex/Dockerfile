FROM ubuntu:22.04
RUN apt-get update
RUN apt-get install -y python3 python3-pip
WORKDIR /home/app
ADD requirements.txt .
RUN pip install -r requirements.txt
ENTRYPOINT ["python3"]
CMD ["src/main.py"]