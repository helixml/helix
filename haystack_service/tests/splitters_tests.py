import os
import base64
from unittest import TestCase
import tempfile

from haystack import Document
from haystack.dataclasses.byte_stream import ByteStream
from app.splitters import ImageSplitter


class TestImageSplitter(TestCase):
    def setUp(self):
        self.splitter = ImageSplitter()
        # Create temporary test images
        self.temp_dir = tempfile.mkdtemp()

        # Create a small test PNG file
        self.test_png_path = os.path.join(self.temp_dir, "test.png")
        with open(self.test_png_path, "wb") as f:
            f.write(b"fake PNG data")

        # Create a small test JPG file
        self.test_jpg_path = os.path.join(self.temp_dir, "test.jpg")
        with open(self.test_jpg_path, "wb") as f:
            f.write(b"fake JPG data")

    def tearDown(self):
        # Clean up temporary files
        for file in [self.test_png_path, self.test_jpg_path]:
            if os.path.exists(file):
                os.remove(file)
        os.rmdir(self.temp_dir)

    def test_empty_documents(self):
        """Test that empty input returns empty output"""
        result = self.splitter.run([])
        self.assertEqual(result["documents"], [])

    def test_document_without_images(self):
        """Test handling of document without image_paths metadata"""
        doc = Document(content="test content", meta={"some": "metadata"})
        result = self.splitter.run([doc])

        self.assertEqual(len(result["documents"]), 1)
        output_doc = result["documents"][0]
        self.assertEqual(output_doc.content, "test content")
        self.assertEqual(output_doc.meta["some"], "metadata")
        self.assertEqual(output_doc.meta["page_number"], 0)
        self.assertEqual(output_doc.meta["source_id"], doc.id)
        self.assertEqual(output_doc.meta["original_document_id"], doc.id)

    def test_document_with_images(self):
        """Test splitting document with multiple images"""
        doc = Document(
            content="test content",
            meta={
                "some": "metadata",
                "image_paths": [self.test_png_path, self.test_jpg_path],
            },
        )
        result = self.splitter.run([doc])

        self.assertEqual(len(result["documents"]), 2)

        # Check first document (PNG)
        png_doc = result["documents"][0]
        with open(self.test_png_path, "rb") as f:
            expected_png_data = base64.b64encode(f.read()).decode("utf-8")

        self.assertEqual(png_doc.content, expected_png_data)
        self.assertEqual(png_doc.meta["some"], "metadata")
        self.assertEqual(png_doc.meta["page_number"], 1)
        self.assertEqual(png_doc.meta["source_id"], doc.id)
        self.assertEqual(png_doc.meta["original_document_id"], doc.id)
        self.assertEqual(png_doc.meta["image_path"], self.test_png_path)
        self.assertIsInstance(png_doc.blob, ByteStream)
        self.assertEqual(png_doc.blob.mime_type, "image/png")

        # Check second document (JPG)
        jpg_doc = result["documents"][1]
        with open(self.test_jpg_path, "rb") as f:
            expected_jpg_data = base64.b64encode(f.read()).decode("utf-8")

        self.assertEqual(jpg_doc.content, expected_jpg_data)
        self.assertEqual(jpg_doc.meta["some"], "metadata")
        self.assertEqual(jpg_doc.meta["page_number"], 2)
        self.assertEqual(jpg_doc.meta["source_id"], doc.id)
        self.assertEqual(jpg_doc.meta["original_document_id"], doc.id)
        self.assertEqual(jpg_doc.meta["image_path"], self.test_jpg_path)
        self.assertIsInstance(jpg_doc.blob, ByteStream)
        self.assertEqual(jpg_doc.blob.mime_type, "image/jpeg")

    def test_multiple_documents(self):
        """Test processing multiple documents at once"""
        doc1 = Document(content="doc1", meta={"image_paths": [self.test_png_path]})
        doc2 = Document(content="doc2", meta={"image_paths": [self.test_jpg_path]})

        result = self.splitter.run([doc1, doc2])

        self.assertEqual(len(result["documents"]), 2)
        self.assertEqual(result["documents"][0].meta["original_document_id"], doc1.id)
        self.assertEqual(result["documents"][1].meta["original_document_id"], doc2.id)
