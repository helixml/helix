#!/bin/bash

# Create directory and symlink if necessary
mkdir -p /data/models
if [ ! -e /data/models/all-MiniLM-L12-v2 ]; then
    ln -s /models/all-MiniLM-L12-v2 /data/models/all-MiniLM-L12-v2
fi

# Execute the Typesense server with all passed arguments
exec /opt/typesense-server "$@"
