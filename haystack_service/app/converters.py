import logging
from typing import List, Dict, Any, Optional

from haystack import Document
from unstructured.partition.auto import partition
from unstructured.documents.elements import (
    Title, ListItem, Header, Footer, Table, Image
)

logger = logging.getLogger(__name__)

class LocalUnstructuredConverter:
    """Converts documents to text using unstructured local library"""
    
    def _element_to_markdown(self, element) -> str:
        """Convert an unstructured element to markdown format"""
        if not str(element).strip():
            return ""
            
        text = str(element).strip()
        
        if isinstance(element, Title):
            return f"# {text}"
        elif isinstance(element, Header):
            return f"## {text}"
        elif isinstance(element, ListItem):
            return f"- {text}"
        elif isinstance(element, Table):
            # Basic table formatting - could be enhanced
            return f"**Table**: {text}"
        elif isinstance(element, Image):
            return f"![Image]{text}"
        elif isinstance(element, Footer):
            return f"*{text}*"
        else:
            # NarrativeText, Text, etc
            return text
    
    def run(self, paths: List[str], meta: Optional[Dict[str, Any]] = None) -> Dict[str, List[Document]]:
        """Convert files to Haystack Documents using local unstructured library
        
        Args:
            paths: List of file paths to process
            meta: Optional metadata to attach to documents
            
        Returns:
            Dict containing list of converted Documents
        """
        if meta is None:
            meta = {}
            
        documents = []
        for path in paths:
            try:
                # Use local partition() directly
                elements = partition(filename=path)
                
                # Convert elements to markdown
                markdown_elements = [
                    self._element_to_markdown(el) for el in elements
                ]
                
                # Filter out empty strings and join with double newlines
                text = "\n\n".join(el for el in markdown_elements if el)
                
                if text.strip():
                    # Create document with metadata
                    doc_meta = meta.copy()
                    doc_meta["file_path"] = path
                    documents.append(Document(content=text, meta=doc_meta))
                    logger.info(f"Extracted {len(text)} characters from {path}")
                else:
                    logger.warning(f"No text extracted from file: {path}")
                    
            except Exception as e:
                logger.error(f"Failed to process file {path}: {str(e)}")
                
        return {"documents": documents} 