# Deploying Helix control plane and a runner to GCP

We are going to use an NVIDIA L4 GPU which has 24GB RAM.


## Requirements

* Google Cloud account
* `gcloud` CLI installed and authenticated
  * e.g `brew install google-cloud-sdk` on macOS, or [see here](https://cloud.google.com/sdk/docs/install)
  * `gcloud auth login`
  <!-- * `gcloud auth application-default login` (is this needed?) -->
* Sufficient quota to deploy one `n2-standard-2` and a `g2-standard-4` with an NVIDIA L4 GPU.
  * You may need to request additional quota to deploy GPUs in GCP.
  * Check "all quotas" and check "gpus_all_regions" and "nvidia_l4_gpus" in your regions of choice.

## Set variables

```
export PROJECT_ID=<your-gcp-project-id>
```

Ensure this is the default for following commands:
```
gcloud config set project $PROJECT_ID
```

You can get see your project ID in the [GCP console](https://console.cloud.google.com/).

Then set `ZONE`. It will default to `us-central1-a`, but the variable is needed for the `gcloud` commands below.

```
export ZONE=us-central1-a
```

## Deploy instances

Run:
```
./01_deploy_instances.sh
```

This will deploy the two instances to your GCP account.


## Install Control Plane

To log into the controlplane VM and install the Helix control plane, use the following command:

```
gcloud compute ssh --zone "$ZONE" "runner" --project "$PROJECT_ID"
```





## Tear down

Run:
```
./99_cleanup.sh
```

This will destroy all GCP resources created by 01_deploy_instances.sh.
