#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10,<3.13"
# dependencies = [
#     "optimum>=2.0",
#     "optimum-onnx>=0.1.0",
#     "onnxruntime>=1.17",
#     "transformers>=4.49",
#     "torch>=2.6",
# ]
# ///
"""Convert st-codesearch-distilroberta-base to ONNX format for hugot.

This is used during Docker image builds to prepare the embedding model.
The model files are placed at {output}/flax-sentence-embeddings_st-codesearch-distilroberta-base/.
"""
import os
import shutil
import sys

from optimum.onnxruntime import ORTModelForFeatureExtraction
from transformers import AutoTokenizer

MODEL_ID = "flax-sentence-embeddings/st-codesearch-distilroberta-base"
DEFAULT_OUTPUT = "models/flax-sentence-embeddings_st-codesearch-distilroberta-base"


def main():
    output_dir = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_OUTPUT

    # Skip if already converted
    onnx_dest = os.path.join(output_dir, "onnx", "model.onnx")
    if os.path.exists(onnx_dest):
        print(f"Model already converted at {output_dir}")
        return

    # Export to ONNX
    model = ORTModelForFeatureExtraction.from_pretrained(MODEL_ID, export=True)
    tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)

    os.makedirs(os.path.join(output_dir, "onnx"), exist_ok=True)
    model.save_pretrained(output_dir)
    tokenizer.save_pretrained(output_dir)

    # hugot expects onnx/model.onnx -- move if optimum places it at top level
    onnx_file = os.path.join(output_dir, "model.onnx")
    if os.path.exists(onnx_file) and not os.path.exists(onnx_dest):
        shutil.move(onnx_file, onnx_dest)

    print(f"Model converted to {output_dir}")


if __name__ == "__main__":
    main()
