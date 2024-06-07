import os
from unstructured.partition.auto import partition
import tempfile
import requests
from bs4 import BeautifulSoup
import html2text
from utils import HttpException

def download_url(url):
  response = requests.get(url)
  if response.status_code == 200:
    temp_file = tempfile.NamedTemporaryFile(delete=False)
    temp_file.write(response.content)
    temp_file.close()
    return temp_file.name, response.headers.get('Content-Type')
  else:
    raise HttpException(f"Download failed with {url} {response.status_code}: {response.text}", response.status_code)

# set to false to use html2text
USE_BEAUTIFUL_SOUP = False

def parse_document_url(url):

  # download url to temporary location
  fname, mimeType = download_url(url)

  print(f"Got mimeType {mimeType}")
  if mimeType.startswith("text/html"):
    if USE_BEAUTIFUL_SOUP:
      # beautiful soup does a better job than unstructured on html
      gfg = BeautifulSoup(open(fname).read())

      maybeArticle = gfg.find('article')
      if maybeArticle:
        # Extracting data for article section
        bodyHtml = maybeArticle
      else:
        bodyHtml = gfg
  
      # Calculating result
      res = bodyHtml.get_text()

      os.unlink(fname)
      return res
    else:
      # but html2text does an even better job (and outputs markdown which LLMs
      # like)
      h = html2text.HTML2Text()
      h.ignore_links = True
      h.body_width = 0
      h.images_to_alt = True
      return h.handle(open(fname).read())


  # otherwise fall back to unstructured
  elements = partition(filename=fname)
  text = ""
  for element in elements:
    if hasattr(element, "text"):
      text += element.text + "\n"

  os.unlink(fname)
  return text

# parse_document_content uses unstructured to parse the content
# such as PDFs, DOCX, etc.
def parse_document_content(content):
  temp_file = tempfile.NamedTemporaryFile(delete=False)
  temp_file.write(content)
  temp_file.close()
  
  elements = partition(filename=temp_file.name)
  text = ""
  for element in elements:
    if hasattr(element, "text"):
      text += element.text + "\n"

  os.unlink(temp_file.name)
  return text