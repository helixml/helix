import logging
from typing import List, Dict, Any, Optional

from haystack import component, Document
from unstructured.partition.auto import partition
from unstructured.documents.elements import (
    Title, ListItem, Header, Footer, Table, Image
)

logger = logging.getLogger(__name__)

@component
class LocalUnstructuredConverter:
    """Converts documents to text using unstructured local library"""
    
    def _element_to_markdown(self, element) -> str:
        """Convert an unstructured element to markdown format"""
        if not str(element).strip():
            return ""
            
        text = str(element).strip()
        
        if isinstance(element, Title):
            return f"# {text}"
        elif isinstance(element, ListItem):
            return f"- {text}"
        elif isinstance(element, Table):
            # Basic table formatting - could be enhanced
            return f"**Table**: {text}"
        elif isinstance(element, Image):
            return f"![Image]{text}"
        elif isinstance(element, Header):
            # strip these
            pass
        elif isinstance(element, Footer):
            pass
        else:
            # NarrativeText, Text, etc
            return text
    
    def _read_text_file(self, path: str) -> str:
        """Read text files directly"""
        try:
            with open(path, 'r', encoding='utf-8') as f:
                return f.read()
        except UnicodeDecodeError:
            # Try with a different encoding if UTF-8 fails
            with open(path, 'r', encoding='latin-1') as f:
                return f.read()
    
    @component.output_types(documents=List[Document])
    def run(
        self,
        paths: List[str],
        meta: Optional[Dict[str, Any]] = None
    ) -> Dict[str, List[Document]]:
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
                # For text files, read directly without using unstructured
                if path.lower().endswith(('.txt', '.md')):
                    text = self._read_text_file(path)
                else:
                    # Use unstructured for other file types
                    elements = partition(filename=path)
                    markdown_elements = [
                        self._element_to_markdown(el) for el in elements
                    ]
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
                raise RuntimeError(f"Failed to process file {path}: {str(e)}")
                
        if not documents:
            raise RuntimeError("No documents were successfully processed")
            
        return {"documents": documents} 