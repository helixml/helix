import os
from llama_index.embeddings import HuggingFaceEmbedding

####################
#
# CONFIG
#
####################

HUGGINGFACE_EMBEDDING_NAME = os.getenv("HUGGINGFACE_EMBEDDING_NAME", "BAAI/bge-small-en")
embed_model = HuggingFaceEmbedding(model_name=HUGGINGFACE_EMBEDDING_NAME)

def getEmbedding(text):
  return embed_model.get_text_embedding(text)