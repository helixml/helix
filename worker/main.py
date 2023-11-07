import os
import argparse
import sys
from llama_index import ServiceContext
from llama_index.llms import OpenAI
from pathlib import Path
from llama_hub.file.pymu_pdf.base import PyMuPDFReader
from llama_index import Document
from llama_index.callbacks import CallbackManager
from llama_index.evaluation import DatasetGenerator
from llama_index.node_parser import SimpleNodeParser
from llama_index.indices.list.base import SummaryIndex
import json

def extract_pdf(filepath):
    loader = PyMuPDFReader()
    docs0 = loader.load(file_path=Path(filepath))
    doc_text = "\n\n".join([d.get_content() for d in docs0])
    metadata = {
        "paper_title": filepath
    }
    return Document(text=doc_text, metadata=metadata)

def extract_file(filepath):
    # Check if file is a PDF
    if os.path.splitext(filepath)[1].lower() == ".pdf":
        return extract_pdf(filepath)
    else:
        # TODO: Implement text extraction for other file types
        raise NotImplementedError("Text extraction not implemented yet")

if __name__ == "__main__":
    openAIAPIKey = os.environ.get("OPENAI_API_KEY", "")
    
    if openAIAPIKey == "":
        sys.exit("OPENAI_API_KEY is not set")

    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=str, required=True)
    parser.add_argument("--output", type=str, required=True)
    args = parser.parse_args()
    if not os.path.isfile(args.input):
      raise ValueError(f"File {args.input} does not exist.")
    
    doc = extract_file(args.input)
    print("Text Extracted")
    print(doc.get_content())
    callback_manager = CallbackManager([])
    gpt_35_context = ServiceContext.from_defaults(
        llm=OpenAI(model="gpt-3.5-turbo-0613", temperature=0.3),
        callback_manager=callback_manager,
    )
    gpt_4_context = ServiceContext.from_defaults(
        llm=OpenAI(model="gpt-4-0613", temperature=0.3),
        callback_manager=callback_manager,
    )

    node_parser = SimpleNodeParser.from_defaults()
    nodes = node_parser.get_nodes_from_documents([doc])

    num_questions_per_chunk = 10
    question_gen_query = (
        "You are a Teacher/ Professor. Your task is to setup a quiz/examination."
        f" Using the provided context, formulate {num_questions_per_chunk} that"
        " captures an important fact from the context. \nYou MUST obey the"
        " following criteria:\n- Restrict the question to the context information"
        " provided.\n- Do NOT create a question that cannot be answered from the"
        " context.\n- Phrase the question so that it does NOT refer to specific"
        ' context. For instance, do NOT put phrases like "given provided context"'
        ' or "in this work" in the question, because if the question is asked'
        " elsewhere it wouldn't be provided specific context. Replace these"
        " terms with specific details.\nBAD questions:\nWhat did the author do in"
        " his childhood\nWhat were the main findings in this report\n\nGOOD"
        " questions:\nWhat did Barack Obama do in his childhood\nWhat were the"
        " main findings in the original Transformers paper by Vaswani et"
        " al.\n\nGenerate the questions below:\n"
    )

    fp = open(args.output, "w")
    for idx, node in enumerate(nodes):
        dataset_generator = DatasetGenerator(
            [node],
            question_gen_query=question_gen_query,
            service_context=gpt_4_context,
            metadata_mode="all",
        )
        node_questions_0 = dataset_generator.generate_questions_from_nodes(num=10)
        print(f"[Node {idx} of {len(nodes)}] Generated questions:\n {node_questions_0}")
        # for each question, get a response
        for question in node_questions_0:
            index = SummaryIndex([node], service_context=gpt_35_context)
            query_engine = index.as_query_engine()
            response = query_engine.query(question)
            out_dict = {"query": question, "response": str(response)}
            print(f"[Node {idx}] Outputs: {out_dict}")
            fp.write(json.dumps(out_dict) + "\n")

    fp.close()

      