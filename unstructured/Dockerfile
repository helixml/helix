FROM downloads.unstructured.io/unstructured-io/unstructured:latest
RUN mkdir /home/notebook-user/app
WORKDIR /home/notebook-user/app
ADD . /home/notebook-user/app
RUN pip install flask
RUN pip install beautifulsoup4 html2text
ENTRYPOINT ["python3"]
CMD ["src/main.py"]