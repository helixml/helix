from typing import Dict, List, Any, Optional
import base64
from haystack import Document, component, logging
from haystack.dataclasses.byte_stream import ByteStream


logger = logging.getLogger(__name__)


@component
class ImageSplitter:
    """
    A component that splits documents containing multiple PNG images into individual documents.

    Each input document is expected to have image data in its content field or meta field.
    The component creates a new document for each image, preserving the original metadata
    and adding source_id and page_number to track the original document and page position.
    """

    def __init__(self):
        """
        Initialize the ImageSplitter component.
        """
        pass

    @component.output_types(documents=List[Document])
    def run(self, documents: List[Document]) -> Dict[str, Any]:
        """
        Split documents containing multiple images into individual documents.

        Args:
            documents: A list of documents, each potentially containing multiple images.

        Returns:
            Dict with 'documents' key containing a list of documents, where each document
            represents a single image/page.
        """
        if not documents:
            return {"documents": []}

        split_documents = []
        for doc_index, doc in enumerate(documents):
            # Check if the document has pages in its metadata
            if "image_paths" in doc.meta and isinstance(doc.meta["image_paths"], list):
                image_paths = doc.meta["image_paths"]

                for page_index, image_path in enumerate(image_paths):
                    # Read the image data and write to a base64 string
                    with open(image_path, "rb") as image_file:
                        image_data = image_file.read()
                        base64_image_data = base64.b64encode(image_data).decode("utf-8")

                    mime_type = (
                        "image/png"
                        if image_path.lower().endswith(".png")
                        else "image/jpeg"
                        if image_path.lower().endswith((".jpg", ".jpeg"))
                        else "application/octet-stream"
                    )

                    # Add the correct base64 header to the image data
                    base64_image_data = f"data:{mime_type};base64,{base64_image_data}"

                    # Create byte stream with proper mime type detection
                    byte_stream = ByteStream(
                        data=image_data,
                        meta={},
                        mime_type=mime_type,
                    )

                    # Create a haystack Document with proper ByteStream
                    page_doc = Document(
                        content=base64_image_data,  # Must set this as well because the Helix API expects there to be content
                        blob=byte_stream,
                        meta={
                            **doc.meta,
                            "source_id": doc.id,
                            "page_number": page_index + 1,
                            "original_document_id": doc.id,
                            "image_path": image_path,
                        },
                    )

                    # Remove the image_paths array from the metadata to avoid duplication
                    if "image_paths" in page_doc.meta:
                        del page_doc.meta["image_paths"]

                    split_documents.append(page_doc)

                logger.debug(
                    f"Split document {doc.id} into {len(image_paths)} documents"
                )
            else:
                # If the document doesn't have pages metadata, just add it as is
                # with appropriate metadata to maintain consistency
                doc.meta["source_id"] = doc.id
                doc.meta["page_number"] = 0
                doc.meta["original_document_id"] = doc.id
                split_documents.append(doc)

                logger.debug(
                    f"Document {doc.id} has no pages metadata, treating as single page"
                )

        logger.info(
            f"successfully split {len(documents)} input documents into {len(split_documents)} output documents"
        )
        return {"documents": split_documents}
