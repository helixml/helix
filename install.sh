#!/bin/bash

# Install:
# curl -LO https://get.helix.ml/install.sh && chmod +x install.sh

set -euo pipefail

echo -e "\033[1;91m"
echo -ne " ░█░█░█▀▀░█░░░▀█▀░█░█░░░░█▄█░█░░"
echo -ne "\033[0m"
echo -e "\033[1;93m"
echo -ne " ░█▀█░█▀▀░█░░░░█░░▄▀▄░░░░█░█░█░░"
echo -ne "\033[0m"
echo -e "\033[1;92m"
echo -ne " ░▀░▀░▀▀▀░▀▀▀░▀▀▀░▀░▀░▀░░▀░▀░▀▀▀"
echo -e "\033[0m"
echo -e "\033[1;96m              Private GenAI Stack\033[0m"
echo

set -euo pipefail

# Default values
AUTO=true
CLI=false
CONTROLPLANE=false
RUNNER=false
LARGE=false
API_HOST=""
RUNNER_TOKEN=""
TOGETHER_API_KEY=""
OPENAI_API_KEY=""
OPENAI_BASE_URL=""
AUTO_APPROVE=false
OLDER_GPU=false
HF_TOKEN=""
PROXY=https://get.helix.ml
EXTRA_OLLAMA_MODELS=""
HELIX_VERSION=""

# Determine OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Set binary name
BINARY_NAME="helix-${OS}-${ARCH}"

# Set installation directory
if [ "$OS" = "linux" ]; then
    INSTALL_DIR="/opt/HelixML"
elif [ "$OS" = "darwin" ]; then
    INSTALL_DIR="$HOME/HelixML"
fi


# Function to display help message
display_help() {
    cat << EOF
Usage: ./install.sh [OPTIONS]

Options:
  --cli                    Install the CLI (binary in /usr/local/bin)
  --controlplane           Install the controlplane (API, Postgres etc in Docker Compose in $INSTALL_DIR)
  --runner                 Install the runner (single container with runner.sh script to start it in $INSTALL_DIR)
  --large                  Install the large version of the runner (includes all models, 100GB+ download, otherwise uses small one)
  --api-host <host>        Specify the API host for the API to serve on and/or the runner to connect to, e.g. http://localhost:8080 or https://my-controlplane.com. Will install and configure Caddy if HTTPS and running on Ubuntu.
  --runner-token <token>   Specify the runner token when connecting a runner to an existing controlplane
  --together-api-key <token> Specify the together.ai token for inference, rag and apps without a GPU
  --openai-api-key <key>   Specify the OpenAI API key for any OpenAI compatible API
  --openai-base-url <url>  Specify the base URL for the OpenAI API
  --older-gpu              Disable axolotl and sdxl models (which don't work on older GPUs) on the runner
  --hf-token <token>       Specify the Hugging Face token for downloading models
  -y                       Auto approve the installation
  --extra-ollama-models    Specify additional Ollama models to download when installing the runner (comma-separated), for example "nemotron-mini:4b,codegemma:2b-code-q8_0"
  --helix-version <version>  Override the Helix version to install (e.g. 1.4.0-rc4, defaults to latest stable)

Examples:

1. Install the CLI, the controlplane and a runner if a GPU is available (auto mode):
   ./install.sh

2. Install alongside Ollama already running:
   ./install.sh --openai-api-key ollama --openai-base-url http://host.docker.internal:11434/v1

3. Install just the CLI:
   ./install.sh --cli

4. Install CLI and controlplane with external TogetherAI token:
   ./install.sh --cli --controlplane --together-api-key YOUR_TOGETHER_API_KEY

5. Install CLI and controlplane (to install runner separately), specifying a DNS name, automatically setting up TLS:
   ./install.sh --cli --controlplane --api-host https://helix.mycompany.com

6. Install CLI, controlplane, and runner on a node with a GPU:
   ./install.sh --cli --controlplane --runner

7. Install just the runner, pointing to a controlplane with a DNS name (find runner token in /opt/HelixML/.env):
   ./install.sh --runner --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN

8. Install CLI and controlplane with OpenAI-compatible API key and base URL:
   ./install.sh --cli --controlplane --openai-api-key YOUR_OPENAI_API_KEY --openai-base-url YOUR_OPENAI_BASE_URL

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            display_help
            exit 0
            ;;
        --cli)
            CLI=true
            AUTO=false
            shift
            ;;
        --controlplane)
            CONTROLPLANE=true
            AUTO=false
            shift
            ;;
        --runner)
            RUNNER=true
            AUTO=false
            shift
            ;;
        --large)
            LARGE=true
            shift
            ;;
        --api-host=*)
            API_HOST="${1#*=}"
            shift
            ;;
        --api-host)
            API_HOST="$2"
            shift 2
            ;;
        --runner-token=*)
            RUNNER_TOKEN="${1#*=}"
            shift
            ;;
        --runner-token)
            RUNNER_TOKEN="$2"
            shift 2
            ;;
        --together-api-key=*)
            TOGETHER_API_KEY="${1#*=}"
            shift
            ;;
        --together-api-key)
            TOGETHER_API_KEY="$2"
            shift 2
            ;;
        --openai-api-key=*)
            OPENAI_API_KEY="${1#*=}"
            shift
            ;;
        --openai-api-key)
            OPENAI_API_KEY="$2"
            shift 2
            ;;
        --openai-base-url=*)
            OPENAI_BASE_URL="${1#*=}"
            shift
            ;;
        --openai-base-url)
            OPENAI_BASE_URL="$2"
            shift 2
            ;;
        --older-gpu)
            OLDER_GPU=true
            shift
            ;;
        --hf-token=*)
            HF_TOKEN="${1#*=}"
            shift
            ;;
        --hf-token)
            HF_TOKEN="$2"
            shift 2
            ;;
        -y)
            AUTO_APPROVE=true
            shift
            ;;
        --extra-ollama-models=*)
            EXTRA_OLLAMA_MODELS="${1#*=}"
            shift
            ;;
        --extra-ollama-models)
            EXTRA_OLLAMA_MODELS="$2"
            shift 2
            ;;
        --helix-version=*)
            HELIX_VERSION="${1#*=}"
            shift
            ;;
        --helix-version)
            HELIX_VERSION="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            display_help
            exit 1
            ;;
    esac
done

# Determine version to install
if [ -n "$HELIX_VERSION" ]; then
    LATEST_RELEASE="$HELIX_VERSION"
    echo "Using specified Helix version: $LATEST_RELEASE"
else
    LATEST_RELEASE=$(curl -s ${PROXY}/latest.txt)
    echo "Using latest Helix version: $LATEST_RELEASE"
fi


# Function to check for NVIDIA GPU
check_nvidia_gpu() {
    # On windows, WSL2 doesn't support nvidia-smi but docker info can give us a clue
    if command -v nvidia-smi &> /dev/null || sudo docker info 2>/dev/null | grep -i nvidia &> /dev/null; then
        return 0
    else
        return 1
    fi
}


# Function to check if Ollama is running on localhost:11434 or Docker bridge IP
check_ollama() {
    # Check localhost
    if curl -s -o /dev/null -w "%{http_code}" --max-time 5 http://localhost:11434/v1/models >/dev/null; then
        return 0
    fi

    # Check Docker bridge IP
    DOCKER_BRIDGE_IP=$(sudo docker network inspect bridge --format='{{range .IPAM.Config}}{{.Gateway}}{{end}}' 2>/dev/null)
    if [ -n "$DOCKER_BRIDGE_IP" ]; then
        if curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://${DOCKER_BRIDGE_IP}:11434/v1/models" >/dev/null; then
            return 0
        fi
    fi

    return 1
}

# Adjust default values based on provided arguments and AUTO mode
if [ "$AUTO" = true ]; then
    CLI=true
    CONTROLPLANE=true
    if check_nvidia_gpu; then
        RUNNER=true
    fi
    echo -e "Auto-install mode detected. Installing CLI and Control Plane.\n"
    if check_nvidia_gpu; then
        echo "🚀 NVIDIA GPU detected. Runner will be installed locally."
        echo
    elif check_ollama; then
        echo "🦙 Ollama detected. Using local Ollama for inference provider."
        echo
    else
        echo "No NVIDIA GPU or Ollama detected. Ensure Ollama is running if you want to "
        echo "use it for inference. Otherwise, you need to point a DNS name at this server "
        echo "and set --api-host (e.g. --api-host https://helix.mycompany.com) and then "
        echo "connect a separate GPU node to this controlplane."
        echo
        echo "Command will be printed at the end to install runner separately on a GPU node, "
        echo "or pass --together-api-key to connect to together.ai for LLM inference."
        echo "See --help for more options."
        echo
    fi
fi

if [ "$RUNNER" = true ] && [ "$CONTROLPLANE" = false ] && [ -z "$API_HOST" ]; then
    echo "Error: When installing only the runner, you must specify --api-host and --runner-token"
    echo "to connect to an external controlplane, for example:"
    echo
    echo "./install.sh --runner --api-host https://your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN"
    echo
    echo "You can find the runner token in <HELIX_INSTALL_DIR>/.env on the controlplane node."
    exit 1
fi

# Function to gather planned modifications
gather_modifications() {
    local modifications=""

    if [ "$CLI" = true ]; then
        modifications+="  - Install Helix CLI version ${LATEST_RELEASE}\n"
    fi

    if [ "$CONTROLPLANE" = true ] || [ "$RUNNER" = true ]; then
        modifications+="  - Ensure Docker and Docker Compose plugin are installed\n"
    fi

    if [ "$CONTROLPLANE" = true ]; then
        modifications+="  - Install Helix Control Plane version ${LATEST_RELEASE}\n"
    fi

    if [ "$RUNNER" = true ]; then
        modifications+="  - Ensure NVIDIA Docker runtime is installed\n"
        modifications+="  - Install Helix Runner version ${LATEST_RELEASE}\n"
    fi

    echo -e "$modifications"
}

# Function to ask for user approval
ask_for_approval() {
    if [ "$AUTO_APPROVE" = true ]; then
        return 0
    fi

    echo "┌───────────────────────────────────────────────────────────────────────────┐"
    echo "│ The following modifications will be made to your system:                  │"
    echo "└───────────────────────────────────────────────────────────────────────────┘"
    echo
    gather_modifications
    echo "┌───────────────────────────────────────────────────────────────────────────┐"
    echo "│ If this is not what you want, re-run the script with --help at the end to │"
    echo "│ see other options.                                                        │"
    echo "└───────────────────────────────────────────────────────────────────────────┘"
    echo
    read -p "Do you want to proceed? (y/N) " response
    case "$response" in
        [yY][eE][sS]|[yY])
            return 0
            ;;
        *)
            echo "Installation aborted."
            exit 1
            ;;
    esac
}

# Ask for user approval before proceeding
ask_for_approval

sudo mkdir -p $INSTALL_DIR
# Change the owner of the installation directory to the current user
sudo chown -R $(id -un):$(id -gn) $INSTALL_DIR
mkdir -p $INSTALL_DIR/data/helix-{postgres,filestore,pgvector}
mkdir -p $INSTALL_DIR/scripts/postgres/

# Install CLI if requested or in AUTO mode
if [ "$CLI" = true ]; then
    echo -e "\nDownloading Helix CLI..."
    sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o /usr/local/bin/helix
    sudo chmod +x /usr/local/bin/helix
    echo "Helix CLI has been installed to /usr/local/bin/helix"
fi

# Function to generate random alphanumeric password
generate_password() {
    openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16
}

# Function to check if running on WSL2 (don't auto-install docker in that case)
check_wsl2_docker() {
    if grep -qEi "(Microsoft|WSL)" /proc/version &> /dev/null; then
        echo "Detected WSL2 (Windows) environment."
        echo "Please install Docker Desktop for Windows from https://docs.docker.com/desktop/windows/install/"
        exit 1
    fi
}

# Function to install Docker and Docker Compose plugin
install_docker() {
    if ! command -v docker &> /dev/null; then
        check_wsl2_docker
        echo "Docker not found. Installing Docker..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    sudo apt-get update
                    sudo apt-get install -y ca-certificates curl gnupg
                    sudo install -m 0755 -d /etc/apt/keyrings
                    curl -fsSL https://download.docker.com/linux/$ID/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
                    sudo chmod a+r /etc/apt/keyrings/docker.gpg
                    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$ID $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
                    sudo apt-get update
                    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    ;;
                fedora)
                    sudo dnf -y install dnf-plugins-core
                    sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
                    sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    sudo systemctl start docker
                    sudo systemctl enable docker
                    ;;
                *)
                    echo "Unsupported distribution for automatic Docker installation. Please install Docker manually."
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install Docker manually."
            exit 1
        fi
    fi

    if ! docker compose version &> /dev/null; then
        echo "Docker Compose plugin not found. Installing Docker Compose plugin..."
        sudo apt-get update
        sudo apt-get install -y docker-compose-plugin
    fi
}

# Function to install NVIDIA Docker runtime
install_nvidia_docker() {
    if ! check_nvidia_gpu; then
        echo "NVIDIA GPU not detected. Skipping NVIDIA Docker runtime installation."
        return
    fi

    if ! sudo docker info 2>/dev/null | grep -i nvidia &> /dev/null && ! command -v nvidia-container-toolkit &> /dev/null; then
        check_wsl2_docker
        echo "NVIDIA Docker runtime not found. Installing NVIDIA Docker runtime..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
                    curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add -
                    curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list
                    sudo apt-get update
                    sudo apt-get install -y nvidia-docker2
                    sudo systemctl restart docker
                    ;;
                fedora)
                    distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
                    curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.repo | sudo tee /etc/yum.repos.d/nvidia-docker.repo
                    sudo dnf install -y nvidia-docker2
                    sudo systemctl restart docker
                    ;;
                *)
                    echo "Unsupported distribution for automatic NVIDIA Docker runtime installation. Please install NVIDIA Docker runtime manually."
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install NVIDIA Docker runtime manually."
            exit 1
        fi
    fi
}

# Install controlplane if requested or in AUTO mode
if [ "$CONTROLPLANE" = true ]; then
    install_docker
    echo -e "\nDownloading docker-compose.yaml..."
    sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o $INSTALL_DIR/docker-compose.yaml
    echo "docker-compose.yaml has been downloaded to $INSTALL_DIR/docker-compose.yaml"

    # Create database creation script
    cat << EOF > "$INSTALL_DIR/scripts/postgres/postgres-db.sh"
#!/bin/bash

set -e
set -u

function create_user_and_database() {
	local database=\$1
	echo "  Creating database '\$database'"
	psql -v ON_ERROR_STOP=1 --username "\$POSTGRES_USER" <<-EOSQL
	    CREATE DATABASE \$database;
EOSQL
}

if [ -n "\$POSTGRES_DATABASES" ]; then
	echo "Database creation requested: \$POSTGRES_DATABASES"
	for db in $(echo \$POSTGRES_DATABASES | tr ',' ' '); do
		create_user_and_database \$db
	done
	echo "databases created"
fi
EOF
    chmod +x $INSTALL_DIR/scripts/postgres/postgres-db.sh

    # Create .env file
    ENV_FILE="$INSTALL_DIR/.env"
    echo -e "\nCreating/updating .env file..."
    echo

    # Default to localhost if it wasn't passed
    if [ -z "$API_HOST" ]; then
        API_HOST="http://localhost:8080"
    fi

    if [ -f "$ENV_FILE" ]; then
        echo ".env file already exists. Reusing existing secrets."

        # Make a backup copy of the .env file
        DATE=$(date +%Y%m%d%H%M%S)
        cp "$ENV_FILE" "$ENV_FILE-$DATE"
        echo "Backup of .env file created: $ENV_FILE-$DATE"
        echo
        echo "To see what changed, run:"
        echo "diff $ENV_FILE $ENV_FILE-$DATE"
        echo

        KEYCLOAK_ADMIN_PASSWORD=$(grep '^KEYCLOAK_ADMIN_PASSWORD=' "$ENV_FILE" | sed 's/^KEYCLOAK_ADMIN_PASSWORD=//' || generate_password)
        POSTGRES_ADMIN_PASSWORD=$(grep '^POSTGRES_ADMIN_PASSWORD=' "$ENV_FILE" | sed 's/^POSTGRES_ADMIN_PASSWORD=//' || generate_password)
        RUNNER_TOKEN=$(grep '^RUNNER_TOKEN=' "$ENV_FILE" | sed 's/^RUNNER_TOKEN=//' || generate_password)

    else
        echo ".env file does not exist. Generating new passwords."
        KEYCLOAK_ADMIN_PASSWORD=$(generate_password)
        POSTGRES_ADMIN_PASSWORD=$(generate_password)
        RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}
    fi

    # Generate .env content
    cat << EOF > "$ENV_FILE"
# Set passwords
KEYCLOAK_ADMIN_PASSWORD=$KEYCLOAK_ADMIN_PASSWORD
POSTGRES_ADMIN_PASSWORD=$POSTGRES_ADMIN_PASSWORD
RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}

# URLs
KEYCLOAK_FRONTEND_URL=${API_HOST}/auth/
SERVER_URL=${API_HOST}

# Storage
# Uncomment the lines below and create the directories if you want to persist
# direct to disk rather than a docker volume. You may need to set up the
# directory user and group on the filesystem and in the docker-compose.yaml
# file.
#POSTGRES_DATA=$INSTALL_DIR/data/helix-postgres
#FILESTORE_DATA=$INSTALL_DIR/data/helix-filestore
#PGVECTOR_DATA=$INSTALL_DIR/data/helix-pgvector

# Optional integrations:

## LLM provider
EOF

    AUTODETECTED_LLM=false
    # If user hasn't specified LLM provider, check if Ollama is running on localhost:11434
    if [ -z "$OPENAI_API_KEY" ] && [ -z "$OPENAI_BASE_URL" ] && [ -z "$TOGETHER_API_KEY" ]; then
        echo "No LLM provider specified. Checking if Ollama is running on localhost:11434..."
        if check_ollama; then
            echo "Ollama (or another OpenAI compatible API) detected on localhost:11434. Configuring Helix to use it."
            echo
            echo "OPENAI_API_KEY=ollama" >> "$ENV_FILE"
            echo "OPENAI_BASE_URL=http://host.docker.internal:11434/v1" >> "$ENV_FILE"
            echo "INFERENCE_PROVIDER=openai" >> "$ENV_FILE"
            echo "FINETUNING_PROVIDER=openai" >> "$ENV_FILE"
            AUTODETECTED_LLM=true
        else
            # Only warn the user if there's also no GPU
            if ! check_nvidia_gpu; then
                echo
                echo "┌────────────────────────────────────────────────────────────────────────────────────────────────────────"
                echo "│ ⚠️ Ollama not detected on localhost."
                echo "│ "
                echo "│ Note that Helix will be non-functional without an LLM provider or GPU runner attached."
                echo "│ "
                echo "│ You have 4 options:"
                echo "│ "
                echo "│ 1. USE OLLAMA LOCALLY"
                echo "│    If you want to use Ollama, start it and re-run the installer so that it can be detected"
                echo "│ "
                echo "│ 2. ATTACH YOUR OWN NVIDIA GPU(S)"
                echo "│    You can attach a separate node(s) with an NVIDIA GPU as helix runners (instructions printed below)"
                echo "│ "
                echo "│ 3. USE TOGETHER.AI"
                echo "│    You can re-run the installer with --together-api-key (see --help for details)"
                echo "│ "
                echo "│ 4. USE EXTERNAL OPENAI COMPATIBLE LLM"
                echo "│    You can re-run the installer with --openai-api-key and --openai-base-url (see --help for details)"
                echo "└────────────────────────────────────────────────────────────────────────────────────────────────────────"
                echo
            fi
        fi
    fi

    # Add TogetherAI configuration if token is provided
    if [ -n "$TOGETHER_API_KEY" ]; then
        cat << EOF >> "$ENV_FILE"
INFERENCE_PROVIDER=togetherai
FINETUNING_PROVIDER=togetherai
TOGETHER_API_KEY=$TOGETHER_API_KEY
EOF
    fi

    # Add OpenAI configuration if key and base URL are provided
    if [ -n "$OPENAI_API_KEY" ]; then
        cat << EOF >> "$ENV_FILE"
INFERENCE_PROVIDER=openai
FINETUNING_PROVIDER=openai
OPENAI_API_KEY=$OPENAI_API_KEY
EOF
    fi

    if [ -n "$OPENAI_BASE_URL" ]; then
        cat << EOF >> "$ENV_FILE"
OPENAI_BASE_URL=$OPENAI_BASE_URL
EOF
    fi
    # Set default FINETUNING_PROVIDER to helix if neither OpenAI nor TogetherAI are specified
    # TODO: fix this by making the default FINETUNING_PROVIDER to helix in helix itself
    if [ -z "$OPENAI_API_KEY" ] && [ -z "$TOGETHER_API_KEY" ] && [ "$AUTODETECTED_LLM" = false ]; then
        cat << EOF >> "$ENV_FILE"
FINETUNING_PROVIDER=helix
INFERENCE_PROVIDER=helix
EOF
    fi

    # Continue with the rest of the .env file
    cat << EOF >> "$ENV_FILE"

## Analytics
# GOOGLE_ANALYTICS_FRONTEND=
# SENTRY_DSN_FRONTEND=
# SENTRY_DSN_API=

## Notifications
# EMAIL_SMTP_HOST=smtp.example.com
# EMAIL_SMTP_PORT=25
# EMAIL_SMTP_USERNAME=REPLACE_ME
# EMAIL_SMTP_PASSWORD=REPLACE_ME

# EMAIL_MAILGUN_DOMAIN=REPLACE_ME
# EMAIL_MAILGUN_API_KEY=REPLACE_ME
EOF

    CADDY=false
    # Install Caddy if API_HOST is an HTTPS URL and system is Ubuntu
    if [[ "$API_HOST" == https* ]]; then
        if [[ "$OS" != "linux" ]]; then
            echo "Caddy installation is only supported on Ubuntu. Please install and configure Caddy manually (check the install.sh script for details)."
        else
            CADDY=true
            . /etc/os-release
            if [[ "$ID" != "ubuntu" && "$ID" != "debian" ]]; then
                echo "Caddy installation is only supported on Ubuntu. Please install and configure Caddy manually (check the install.sh script for details)."
            else
                echo "Installing Caddy..."
                sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl

                # Check if the keyring file already exists
                if [ ! -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg ]; then
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
                fi

                # Check if the source list file already exists
                if [ ! -f /etc/apt/sources.list.d/caddy-stable.list ]; then
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
                fi

                sudo apt update
                sudo apt install caddy

                # Create Caddyfile
                CADDYFILE="/etc/caddy/Caddyfile"
                echo "Creating Caddyfile..."
                # Strip https:// and port from API_HOST
                CADDY_HOST=$(echo "$API_HOST" | sed -e 's/^https:\/\///' -e 's/:.*//')
                sudo bash -c "cat << EOF > \"$CADDYFILE\"
$CADDY_HOST {
    reverse_proxy localhost:8080
}
EOF"
                # Add OLLAMA_HOST environment variable to ollama.service on Linux
                if [ "$OS" = "linux" ]; then
                    OLLAMA_SERVICE_FILE="/etc/systemd/system/ollama.service"
                    if [ -f "$OLLAMA_SERVICE_FILE" ]; then
                        echo "Detecting Docker bridge IP..."
                        DOCKER_BRIDGE_IP=$(docker network inspect bridge --format='{{range .IPAM.Config}}{{.Gateway}}{{end}}')
                        if [ -n "$DOCKER_BRIDGE_IP" ]; then
                            echo "Adding OLLAMA_HOST environment variable to ollama.service..."
                            sudo sed -i "/^\[Service\]/a Environment=\"OLLAMA_HOST=$DOCKER_BRIDGE_IP\"" "$OLLAMA_SERVICE_FILE"
                            sudo systemctl daemon-reload
                            echo "Restarting Ollama service..."
                            sudo systemctl restart ollama
                            echo "ollama.service has been updated with OLLAMA_HOST=$DOCKER_BRIDGE_IP and restarted."
                        else
                            echo "Warning: Failed to detect Docker bridge IP. Please add 'Environment=\"OLLAMA_HOST=<your-docker-bridge-ip>\"' to the [Service] section of $OLLAMA_SERVICE_FILE manually and restart the service."
                        fi
                    else
                        echo "Warning: $OLLAMA_SERVICE_FILE not found. Please add 'Environment=\"OLLAMA_HOST=<your-docker-bridge-ip>\"' to the [Service] section manually and restart the service."
                    fi
                fi
                echo "Caddyfile has been created at $CADDYFILE"
                echo "Please start Caddy manually after starting the Docker Compose stack:"
            fi
        fi
    fi

    echo ".env file has been created at $ENV_FILE"
    echo
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ ❗ To complete installation, you MUST now:"
    if [ "$API_HOST" != "http://localhost:8080" ]; then
        echo "│"
        echo "│ If you haven't already, set up DNS for your domain:"
        echo "│   - Create an A record for $(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||') pointing to your server's IP address"
    fi
    echo "│"
    echo "│ Start the Helix services by running:"
    echo "│"
    echo "│ cd $INSTALL_DIR"
    echo "│ sudo docker compose up -d --remove-orphans"
    if [ "$CADDY" = true ]; then
        echo "│ sudo systemctl restart caddy"
    fi
    echo "│"
    echo "│ to start/upgrade Helix.  Helix will be available at $API_HOST"
    echo "│ This will take a minute or so to boot."
    echo "└───────────────────────���───────────────────────────────────────────────────"
fi

# Install runner if requested or in AUTO mode with GPU
if [ "$RUNNER" = true ]; then
    install_docker
    install_nvidia_docker
    # Check for NVIDIA GPU
    if ! check_nvidia_gpu; then
        echo "NVIDIA GPU not detected. Skipping runner installation."
        echo "Set up a runner separately, per https://docs.helix.ml/helix/private-deployment/controlplane/#attaching-a-runner"
        exit 1
    fi

    # Determine GPU memory
    GPU_MEMORY=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | awk '{print int($1/1024)}' || echo "")
    if [ -z "$GPU_MEMORY" ]; then
        echo "Failed to determine GPU memory."
        read -p "Please specify the GPU memory in GB: " GPU_MEMORY
    fi

    # Determine runner tag
    if [ "$LARGE" = true ]; then
        RUNNER_TAG="${LATEST_RELEASE}-large"
    else
        RUNNER_TAG="${LATEST_RELEASE}-small"
    fi

    # Determine runner token
    if [ -z "$RUNNER_TOKEN" ]; then
        if [ -f "$INSTALL_DIR/.env" ]; then
            RUNNER_TOKEN=$(grep RUNNER_TOKEN $INSTALL_DIR/.env | cut -d '=' -f2)
        else
            echo "Error: RUNNER_TOKEN not found in .env file and --runner-token not provided."
            echo "Please provide the runner token using the --runner-token argument."
            exit 1
        fi
    fi

    # Create runner.sh
    cat << EOF > $INSTALL_DIR/runner.sh
#!/bin/bash

# Configuration variables
RUNNER_TAG="${RUNNER_TAG}"
API_HOST="${API_HOST}"
GPU_MEMORY="${GPU_MEMORY}"
RUNNER_TOKEN="${RUNNER_TOKEN}"
OLDER_GPU="${OLDER_GPU:-false}"
HF_TOKEN="${HF_TOKEN}"
EXTRA_OLLAMA_MODELS="${EXTRA_OLLAMA_MODELS}"

# Set older GPU parameter
if [ "\$OLDER_GPU" = "true" ]; then
    OLDER_GPU_PARAM="-e RUNTIME_AXOLOTL_ENABLED=false"
else
    OLDER_GPU_PARAM=""
fi

# Set HF_TOKEN parameter
if [ -n "\$HF_TOKEN" ]; then
    HF_TOKEN_PARAM="-e HF_TOKEN=\$HF_TOKEN"
else
    HF_TOKEN_PARAM=""
fi

# Set EXTRA_OLLAMA_MODELS parameter
if [ -n "\$EXTRA_OLLAMA_MODELS" ]; then
    EXTRA_OLLAMA_MODELS_PARAM="-e RUNTIME_OLLAMA_WARMUP_MODELS=\$EXTRA_OLLAMA_MODELS"
else
    EXTRA_OLLAMA_MODELS_PARAM=""
fi

# Check if api-1 container is running
if sudo docker ps --format '{{.Image}}' | grep 'registry.helix.ml/helix/controlplane'; then
    API_HOST="http://api:80"
    echo "Detected controlplane container running. Setting API_HOST to \${API_HOST}"
fi

# Check if helix_default network exists, create it if it doesn't
if ! sudo docker network inspect helix_default >/dev/null 2>&1; then
    echo "Creating helix_default network..."
    sudo docker network create helix_default
else
    echo "helix_default network already exists."
fi

# Run the docker container
sudo docker run --privileged --gpus all --shm-size=10g \\
    --restart=always -d \\
    --name helix-runner --ipc=host --ulimit memlock=-1 \\
    --ulimit stack=67108864 \\
    --network="helix_default" \\
    -v \${HOME}/.cache/huggingface:/root/.cache/huggingface \\
    \${OLDER_GPU_PARAM} \\
    \${HF_TOKEN_PARAM} \\
    \${EXTRA_OLLAMA_MODELS_PARAM} \\
    registry.helix.ml/helix/runner:\${RUNNER_TAG} \\
    --api-host \${API_HOST} --api-token \${RUNNER_TOKEN} \\
    --runner-id \$(hostname) \\
    --memory \${GPU_MEMORY}GB \\
    --allow-multiple-copies
EOF

    sudo chmod +x $INSTALL_DIR/runner.sh
    echo "Runner script has been created at $INSTALL_DIR/runner.sh"
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ To start the runner, run:"
    echo "│"
    echo "│   sudo $INSTALL_DIR/runner.sh"
    echo "│"
    echo "└───────────────────────────────────────────────────────────────────────────"
fi

if [ -n "$API_HOST" ] && [ "$CONTROLPLANE" = true ]; then
    echo
    echo "To connect an external runner to this controlplane, run on a node with a GPU:"
    echo
    echo "curl -Ls -O https://get.helix.ml/install.sh"
    echo "chmod +x install.sh"
    echo "./install.sh --runner --api-host $API_HOST --runner-token $RUNNER_TOKEN"
fi

echo -e "\nInstallation complete."
