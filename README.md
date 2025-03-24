<div align="center">
<img alt="logo" src="https://tryhelix.ai/assets/img/CDfWIfha3--900.webp" width="250px">

<br/>
<br/>

</div>

<p align="center">
  <a href="https://app.tryhelix.ai/">SaaS</a> •
  <a href="https://docs.helix.ml/docs/controlplane">Private Deployment</a> •
  <a href="https://docs.helix.ml/docs/overview">Docs</a> •
  <a href="https://discord.gg/VJftd844GE">Discord</a>
</p>


# HelixML

[![sbomified](https://sbomify.com/assets/images/logo/badge.svg)](https://app.sbomify.com/project/YKA8fn8v2Q)

[👥 Discord](https://discord.gg/VJftd844GE)

Private GenAI stack. Deploy the best of open AI in your own data center or VPC and retain complete data security & control.

Including support for RAG, API-calling and fine-tuning models that's as easy as drag'n'drop. Build and deploy [LLM apps by writing a helix.yaml](https://docs.helix.ml/helix/develop/getting-started/).

Looking for a private GenAI platform? From language models to image models and more, Helix brings the best of open source AI to your business in an ergonomic, scalable way, while optimizing the tradeoff between GPU memory and latency.

## Install on Docker

Use our quickstart installer:

```
curl -sL -O https://get.helix.ml/install.sh
chmod +x install.sh
sudo ./install.sh
```
The installer will prompt you before making changes to your system. By default, the dashboard will be available on `http://localhost:8080`.

For setting up a deployment with a DNS name, see `./install.sh --help` or read [the detailed docs](https://docs.helix.ml/helix/private-deployment/controlplane/). We've documented easy TLS termination for you.

Attach your own GPU runners per [runners docs](https://docs.helix.ml/helix/private-deployment/controlplane/#attach-a-runner-to-an-existing-control-plane) or use any [external OpenAI-compatible LLM](https://docs.helix.ml/helix/private-deployment/controlplane/#install-control-plane-pointing-at-any-openai-compatible-api).

## Install on Kubernetes

Use our helm charts:
* [Control Plane helm chart](https://docs.helix.ml/helix/private-deployment/helix-controlplane-helm-chart/)
* [Runner helm chart](https://docs.helix.ml/helix/private-deployment/helix-runner-helm-chart/)

## Developer Instructions

For local development, refer to the [Helix local development guide](./local-development.md).

## License

Helix is [licensed](https://github.com/helixml/helix/blob/main/LICENSE.md) under a similar license to Docker Desktop. You can run the source code (in this repo) for free for:

* **Personal Use:** individuals or people personally experimenting
* **Educational Use:** schools/universities
* **Small Business Use:** companies with under $10M annual revenue and less than 250 employees

If you fall outside of these terms, please [contact us](mailto:founders@helix.ml) to discuss purchasing a license for large commercial use. If you are an individual at a large company interested in experimenting with Helix, that's fine under Personal Use until you deploy to more than one GPU on company-owned or paid-for infrastructure.

You are not allowed to use our code to build a product that competes with us.

Contributions to the source code are welcome, and by contributing you confirm that your changes will fall under the same license and be owned by HelixML, Inc.


### Why these clauses in your license?

* We generate revenue to support the development of Helix. We are an independent software company.
* We don't want cloud providers to take our open source code and build a rebranded service on top of it.

If you would like to use some part of this code under a more permissive license, please [get in touch](mailto:founders@helix.ml).

# Helix Knowledge Indexer Metadata Support

The Helix Knowledge Indexer supports adding custom metadata to documents, which can be used to enhance search results and provide additional context for the documents in your knowledge base.

## How Metadata Works

When you upload documents to Helix for knowledge indexing, you can provide additional metadata about each document by creating a companion `.metadata.yaml` file. This metadata is preserved throughout the indexing process and becomes available during retrieval.

### Creating Metadata Files

For any document in your filestore, you can create a metadata YAML file with the same name as the document plus the `.metadata.yaml` extension:

```
document.pdf           # Original document
document.pdf.metadata.yaml  # Metadata file for the document
```

### Metadata File Format

The metadata file supports two formats:

1. **Direct key-value format:**

```yaml
source_url: "https://example.com/original-document"
author: "John Doe"
created_date: "2023-04-15"
category: "Technical Documentation"
```

2. **Nested format with metadata field:**

```yaml
metadata:
  source_url: "https://example.com/original-document"
  author: "John Doe"
  created_date: "2023-04-15"
  category: "Technical Documentation"
```

Both formats are supported. The indexer will first try to parse with the nested format, and if that fails, it will try the direct format.

## How Metadata is Preserved During Indexing

1. When the knowledge indexer processes files, it checks for corresponding `.metadata.yaml` files
2. If found, the metadata is loaded and associated with the document
3. During text extraction and chunking, the metadata is preserved and attached to each chunk
4. When the chunks are indexed in the vector database, the metadata is included with each chunk

## Using Metadata in Queries

When querying the knowledge base, the metadata is returned alongside the document content. This allows you to:

1. See the source of the information (especially useful with the `source_url` field)
2. Filter results based on metadata fields
3. Provide additional context to the LLM about the retrieved content

## Example Use Cases

1. **Source Attribution**: Add `source_url` to documents to keep track of where information came from
2. **Document Classification**: Add `category` or `tags` to help organize and filter content
3. **Authorship Information**: Add `author` and `created_date` to track document origins
4. **Version Control**: Add `version` to track document versions in your knowledge base

## Best Practices

1. Keep metadata consistent across similar documents
2. Use descriptive field names that make sense for your use case
3. For web-crawled content that's added to your knowledge base, consider adding metadata files with the original URLs

## Limitations

1. Metadata files must be in YAML format with the `.metadata.yaml` extension
2. The metadata file must be in the same directory as the document it describes
3. Metadata files themselves are not indexed for content (they're only used to add attributes to the main document)
