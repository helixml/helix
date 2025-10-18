<div align="center">
<img alt="logo" src="https://helix.ml/assets/helix-logo.png" width="250px">

<br/>
<br/>

</div>

<p align="center">
  <a href="https://app.helix.ml/">SaaS</a> •
  <a href="https://docs.helixml.tech/docs/controlplane">Private Deployment</a> •
  <a href="https://docs.helixml.tech/docs/overview">Docs</a> •
  <a href="https://discord.gg/VJftd844GE">Discord</a>
</p>

# HelixML - AI Agents on a Private GenAI stack

[👥 Discord](https://discord.gg/VJftd844GE)

Deploy agents in your own data center or VPC and retain complete data security & control.

Including support for RAG, API-calling and vision support. Build and deploy [LLM apps, aka agents by writing a helix.yaml](https://docs.helixml.tech/helix/develop/getting-started/).

Our GPU scheduler packs models efficiently into available GPU memory and dynamically loads and unloads models depending on demand.

## Install on Docker

Use our quickstart installer:

```
curl -sL -O https://get.helixml.tech/install.sh
chmod +x install.sh
sudo ./install.sh
```
The installer will prompt you before making changes to your system. By default, the dashboard will be available on `http://localhost:8080`.

For setting up a deployment with a DNS name, see `./install.sh --help` or read [the detailed docs](https://docs.helixml.tech/helix/private-deployment/controlplane/). We've documented easy TLS termination for you.

Attach your own GPU runners per [runners docs](https://docs.helixml.tech/helix/private-deployment/controlplane/#attach-a-runner-to-an-existing-control-plane) or use any [external OpenAI-compatible LLM](https://docs.helixml.tech/helix/private-deployment/controlplane/#install-control-plane-pointing-at-any-openai-compatible-api).

## Server Configuration

You can find all environment variables here: [config.go](https://github.com/helixml/helix/blob/main/api/pkg/config/config.go).

## Install on Kubernetes

Use our helm charts:
* [Control Plane helm chart](https://docs.helixml.tech/helix/private-deployment/helix-controlplane-helm-chart/)
* [Runner helm chart](https://docs.helixml.tech/helix/private-deployment/helix-runner-helm-chart/)

## Developer Instructions

For local development, refer to the [Helix local development guide](./local-development.md).

## License

Helix is [licensed](https://github.com/helixml/helix/blob/main/LICENSE.md) under a similar license to Docker Desktop. You can run the source code (in this repo) for free for:

* **Personal Use:** individuals or people personally experimenting
* **Educational Use:** schools/universities
* **Small Business Use:** companies with under $10M annual revenue and less than 250 employees

If you fall outside of these terms, please use the [Launchpad](https://deploy.helix.ml) to purchase a license for large commercial use. Trial licenses are available for experimentation.

You are not allowed to use our code to build a product that competes with us.

Contributions to the source code are welcome, and by contributing you confirm that your changes will fall under the same license and be owned by HelixML, Inc.


### Why these clauses in your license?

* We generate revenue to support the development of Helix. We are an independent software company.
* We don't want cloud providers to take our open source code and build a rebranded service on top of it.

If you would like to use some part of this code under a more permissive license, please [get in touch](mailto:info@helix.ml).
