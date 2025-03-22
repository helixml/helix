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
