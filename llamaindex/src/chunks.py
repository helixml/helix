import os
from unstructured.partition.auto import partition
from unstructured.documents.elements import NarrativeText
from unstructured.chunking.title import chunk_by_title
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

def parse_document(url):

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
    if isinstance(element, NarrativeText):
      text += element.text + "\n"

  os.unlink(fname)
  return text

  # if we want unstructured to do the splitting then we mess with this
  # chunks = chunk_by_title(
  #   elements=elements,
  # )
  # texts = [element.text for element in chunks]
  # return texts
