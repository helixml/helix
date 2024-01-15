from llama_index.embeddings import HuggingFaceEmbedding

####################
#
# CONFIG
#
####################

HUGGINGFACE_EMBEDDING_NAME = "thenlper/gte-small"

embed_model = HuggingFaceEmbedding(model_name="BAAI/bge-small-en")

def getEmbedding(text):
  return embed_model.get_text_embedding(text)