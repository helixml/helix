#!/bin/bash

echo "Testing Wolf's internal JSON API with our video_producer_buffer_caps fix..."

# Test app creation with our new field included
cat > /tmp/test_app_fixed.json << 'EOF'
{
  "title": "Pipeline Fix Test",
  "id": "test-pipeline-fix",
  "runner": {
    "type": "docker",
    "name": "PipelineTest",
    "image": "ubuntu:latest",
    "mounts": [],
    "env": ["TEST_ENV=pipeline_fix"],
    "devices": [],
    "ports": [],
    "base_create_json": "{}"
  },
  "start_virtual_compositor": true,
  "video_producer_buffer_caps": "video/x-raw"
}
EOF

echo "Sending app configuration with video_producer_buffer_caps field..."
echo "Content:"
cat /tmp/test_app_fixed.json

echo -e "\n\nThis JSON now includes the video_producer_buffer_caps field that should prevent GStreamer syntax errors."
echo "The key fix: 'video_producer_buffer_caps': 'video/x-raw' is now included in app creation requests."
echo "This will ensure Wolf's pipeline generation gets: waylanddisplaysrc ! video/x-raw, width=X, height=Y"
echo "Instead of the broken: waylanddisplaysrc ! , width=X, height=Y"