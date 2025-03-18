#!/usr/bin/env python3

import arxiv
import os
import argparse
from pathlib import Path
import logging
from tqdm import tqdm
import time

def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )

def download_papers(num_papers):
    # Create papers directory if it doesn't exist
    papers_dir = Path('papers')
    papers_dir.mkdir(exist_ok=True)
    
    # Configure arxiv client with appropriate delays to be nice to the API
    client = arxiv.Client(
        page_size=100,
        delay_seconds=3,
        num_retries=5
    )
    
    # Create a search query for recent papers in CS and AI
    search = arxiv.Search(
        query="cat:cs.AI OR cat:cs.LG",  # Focus on AI and Machine Learning papers
        max_results=num_papers,
        sort_by=arxiv.SortCriterion.SubmittedDate,
        sort_order=arxiv.SortOrder.Descending
    )
    
    logging.info(f"Downloading {num_papers} papers from arXiv...")
    
    # Download papers with progress bar
    downloaded = 0
    with tqdm(total=num_papers) as pbar:
        try:
            results = list(client.results(search))
            if not results:
                logging.error("No papers found. The search returned empty results.")
                return
                
            logging.info(f"Found {len(results)} papers")
            
            for result in results:
                try:
                    # Create a safe filename from the paper ID and title
                    safe_title = "".join(x for x in result.title if x.isalnum() or x in (' ', '-', '_')).rstrip()
                    filename = f"{result.get_short_id()}_{safe_title[:50]}.pdf"
                    filepath = papers_dir / filename
                    
                    if filepath.exists():
                        logging.debug(f"Skipping existing paper: {filename}")
                        continue
                    
                    # Download the paper
                    result.download_pdf(filename=filepath)
                    logging.info(f"Downloaded: {filename}")
                    downloaded += 1
                    pbar.update(1)
                    
                    # Be nice to the arxiv servers
                    time.sleep(1)
                    
                except Exception as e:
                    logging.error(f"Error downloading paper '{result.title}': {str(e)}")
                    continue
                
                if downloaded >= num_papers:
                    break
                    
        except Exception as e:
            logging.error(f"Error fetching results: {str(e)}")
    
    if downloaded == 0:
        logging.error("Failed to download any papers")
    else:
        logging.info(f"Successfully downloaded {downloaded} papers to {papers_dir.absolute()}")

def main():
    parser = argparse.ArgumentParser(description='Download papers from arXiv')
    parser.add_argument('--papers', type=int, default=10,
                      help='Number of papers to download (default: 10)')
    args = parser.parse_args()
    
    setup_logging()
    download_papers(args.papers)

if __name__ == "__main__":
    main()
