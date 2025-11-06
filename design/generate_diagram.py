
from diagrams import Diagram, Cluster
from diagrams.gcp.network import LoadBalancing, FirewallRules
from diagrams.gcp.compute import ComputeEngine, Functions, Run
from diagrams.gcp.database import SQL, Memorystore
from diagrams.gcp.storage import PersistentDisk
from diagrams.onprem.client import Users
from diagrams.onprem.compute import Server
from diagrams.k8s.network import Ingress

with Diagram("Helix SaaS Platform - GCP Network Architecture", show=False, filename="gcp_architecture"):
    with Cluster("Internet (Untrusted Network)"):
        users = Users("End Users")

    with Cluster("Google Cloud Platform"):
        with Cluster("GCP VPC Network (Private Network: 10.0.0.0/16)"):
            firewall = FirewallRules("GCP Firewall Rules")
            lb = LoadBalancing("Cloud Load Balancer\n- TLS Termination\n- Domain: cloud.helix.ml\n- SSL Certificate Management")

            with Cluster("Google Compute Engine VM"):
                vm = ComputeEngine("Docker Host VM")

                with Cluster("Docker Containers"):
                    ingress = Ingress("Ingress Controller\n- Ingress Class: nginx/gce\n- Path-based Routing")
                    control_plane_container = Run("Control Plane Container")

                    with Cluster("Supporting Services Containers"):
                        postgres = SQL("PostgreSQL (Main DB)")
                        pgvector = SQL("PGVector")
                        typesense = Memorystore("Typesense")
                        gptscript_runner = Functions("GPTScript Runner")
                        chrome_rod = Run("Chrome/Rod")
                        tika = Run("Tika")
                        searxng = Run("SearXNG")

                    with Cluster("Persistent Storage"):
                        postgres_disk = PersistentDisk("PostgreSQL PVC")
                        pgvector_disk = PersistentDisk("PGVector PVC")
                        typesense_disk = PersistentDisk("Typesense PVC")
                        control_plane_disk = PersistentDisk("Control Plane PVC")

    with Cluster("External LLM Providers"):
        openai = Server("OpenAI")
        anthropic = Server("Anthropic")
        togetherai = Server("TogetherAI")

    users >> lb >> ingress >> control_plane_container
    
    control_plane_container >> postgres
    control_plane_container >> pgvector
    control_plane_container >> typesense
    control_plane_container >> gptscript_runner
    control_plane_container >> chrome_rod
    control_plane_container >> tika
    control_plane_container >> searxng

    postgres - postgres_disk
    pgvector - pgvector_disk
    typesense - typesense_disk
    
    control_plane_container >> openai
    control_plane_container >> anthropic
    control_plane_container >> togetherai
