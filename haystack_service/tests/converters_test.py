import os
import pytest
import shutil
import tempfile
from pathlib import Path

from haystack import Document
from app.converters import LocalUnstructuredConverter, PDFToImagesConverter

# Constants for testing
TEST_PDF_PATH = os.path.join(
    os.path.dirname(__file__), "..", "test_files", "sample.pdf"
)
# Create a temporary directory in the system temp directory
TEST_OUTPUT_DIR = tempfile.mkdtemp()


class TestPDFToImagesConverter:
    """Tests for the PDFToImagesConverter component"""

    def setup_method(self):
        """Setup for each test - create output directory"""
        # Ensure the test PDF exists
        assert os.path.exists(TEST_PDF_PATH), f"Test PDF not found at {TEST_PDF_PATH}"

        # Create test output directory
        os.makedirs(TEST_OUTPUT_DIR, exist_ok=True)

    def teardown_method(self):
        """Cleanup after each test - remove output directory"""
        if os.path.exists(TEST_OUTPUT_DIR):
            shutil.rmtree(TEST_OUTPUT_DIR)
        pass

    def test_init_creates_output_dir(self):
        """Test that initializing the converter creates the output directory"""
        # Remove directory if it exists
        if os.path.exists(TEST_OUTPUT_DIR):
            shutil.rmtree(TEST_OUTPUT_DIR)

        PDFToImagesConverter(output_dir=TEST_OUTPUT_DIR)

        # Check directory was created
        assert os.path.exists(TEST_OUTPUT_DIR)
        assert os.path.isdir(TEST_OUTPUT_DIR)

    def test_convert_pdf_to_images(self):
        """Test conversion of PDF to images"""
        # Initialize converter
        converter = PDFToImagesConverter(
            output_dir=TEST_OUTPUT_DIR,
            dpi=150,  # Lower DPI for faster tests
            format="png",
            prefix="test_",
        )

        # Run the converter
        result = converter.run(paths=[TEST_PDF_PATH])

        # Check the documents
        assert len(result["documents"]) == 1
        doc = result["documents"][0]
        assert isinstance(doc, Document)
        assert "PDF converted to" in doc.content

        # Check metadata
        assert "file_path" in doc.meta
        assert doc.meta["file_path"] == TEST_PDF_PATH
        assert "image_paths" in doc.meta
        assert "page_count" in doc.meta
        assert doc.meta["page_count"] == len(
            doc.meta["image_paths"]
        )  # Compare with meta image_paths instead

        # Check all image files exist
        for img_path in doc.meta["image_paths"]:
            assert os.path.exists(img_path)
            assert os.path.isfile(img_path)
            assert img_path.endswith(".png")
            assert Path(img_path).name.startswith("test_")

    def test_multiple_pdfs(self):
        """Test conversion of multiple PDFs"""
        # For this test, we'll use the same PDF twice
        paths = [TEST_PDF_PATH, TEST_PDF_PATH]

        # Initialize converter
        converter = PDFToImagesConverter(output_dir=TEST_OUTPUT_DIR)

        # Run the converter
        result = converter.run(paths=paths)

        # Verify we got documents for each PDF
        assert len(result["documents"]) == 2

        # Verify unique directory structure for each file
        dirs_created = [
            d
            for d in os.listdir(TEST_OUTPUT_DIR)
            if os.path.isdir(os.path.join(TEST_OUTPUT_DIR, d))
        ]
        assert len(dirs_created) == 2  # Should create 2 directories (one for each PDF)

    def test_non_pdf_file(self):
        """Test handling of non-PDF files"""
        # Create a dummy text file
        dummy_file = os.path.join(TEST_OUTPUT_DIR, "dummy.txt")
        with open(dummy_file, "w") as f:
            f.write("This is not a PDF file")

        # Initialize converter
        converter = PDFToImagesConverter(output_dir=TEST_OUTPUT_DIR)

        # Update test to check for empty documents list instead of RuntimeError
        result = converter.run(paths=[dummy_file])
        assert len(result["documents"]) == 0

    def test_custom_metadata(self):
        """Test that custom metadata is preserved and included"""
        # Initialize converter
        converter = PDFToImagesConverter(output_dir=TEST_OUTPUT_DIR)

        # Create custom metadata
        custom_meta = {
            "source": "test",
            "importance": "high",
            "tags": ["sample", "test"],
        }

        # Run the converter with custom metadata
        result = converter.run(paths=[TEST_PDF_PATH], meta=custom_meta)

        # Check the custom metadata was included in the document
        doc = result["documents"][0]
        assert doc.meta["source"] == "test"
        assert doc.meta["importance"] == "high"
        assert doc.meta["tags"] == ["sample", "test"]

        # And original PDF metadata is still there
        assert "file_path" in doc.meta
        assert "image_paths" in doc.meta
        assert "page_count" in doc.meta


class TestLocalUnstructuredConverter:
    """Tests for the LocalUnstructuredConverter component"""

    def setup_method(self):
        """Setup for each test"""
        # Ensure the test PDF exists
        assert os.path.exists(TEST_PDF_PATH), f"Test PDF not found at {TEST_PDF_PATH}"

    def test_pdf_conversion(self):
        """Test conversion of PDF file using unstructured"""
        # Initialize converter
        converter = LocalUnstructuredConverter()

        # Run the converter with the PDF file
        result = converter.run(paths=[TEST_PDF_PATH])

        # Verify the result structure
        assert "documents" in result
        assert len(result["documents"]) == 1

        # Check the document
        doc = result["documents"][0]
        assert isinstance(doc, Document)
        assert doc.content, "Document content should not be empty"

        # Check metadata
        assert "file_path" in doc.meta
        assert doc.meta["file_path"] == TEST_PDF_PATH

    def test_custom_metadata(self):
        """Test that custom metadata is preserved and included"""
        # Initialize converter
        converter = LocalUnstructuredConverter()

        # Create custom metadata
        custom_meta = {
            "source": "test",
            "importance": "high",
            "tags": ["sample", "test"],
        }

        # Run the converter with custom metadata
        result = converter.run(paths=[TEST_PDF_PATH], meta=custom_meta)

        # Check the custom metadata was included in the document
        doc = result["documents"][0]
        assert doc.meta["source"] == "test"
        assert doc.meta["importance"] == "high"
        assert doc.meta["tags"] == ["sample", "test"]

        # And original metadata is still there
        assert "file_path" in doc.meta
        assert doc.meta["file_path"] == TEST_PDF_PATH

    def test_nonexistent_file(self):
        """Test handling of nonexistent files"""
        # Initialize converter
        converter = LocalUnstructuredConverter()

        # Run should fail with a nonexistent file
        with pytest.raises(RuntimeError) as excinfo:
            converter.run(paths=["nonexistent_file.pdf"])

        # Check the error message
        assert "Failed to process file" in str(excinfo.value)

    def test_empty_paths(self):
        """Test handling of empty paths list"""
        # Initialize converter
        converter = LocalUnstructuredConverter()

        # Run should fail as no documents were processed
        with pytest.raises(RuntimeError) as excinfo:
            converter.run(paths=[])

        # Check the error message
        assert "No documents were successfully processed" in str(excinfo.value)
