Wolf API
Wolf exposes a REST API that allows you to interact with the platform programmatically.
The API can be accessed only via UNIX sockets, you can control the exact path by setting the WOLF_SOCKET_PATH environment variable. If you want to access the socket from outside the container, you should mount the socket to the host machine, ex: -e WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock and -v /var/run/wolf:/var/run/wolf will allow you to access the socket from the host machine at /var/run/wolf/wolf.sock.

You can test out the API using the curl command, for example, to get the OpenAPI specification you can run:

curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/openapi-schema
When looking at the examples in the API Reference remember to add the --unix-socket flag to the curl command.

Exposing the API via TCP
Exposing the API is highly dangerous, via the API you can pair clients to the server, execute arbitrary commands, and more.
Make sure to secure the API properly if you decide to expose it.

If you want to expose the API via TCP you can use a reverse proxy like nginx, for example, to expose the API on port 8080 you can use the following config

server {
    listen 8080;

    location / {
        proxy_pass http://unix:/var/run/wolf/wolf.sock;
        proxy_http_version 1.0;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
Save it as wolf.conf and start an Nginx container with the following command:

docker run --name wolf-proxy \
           --network=host \
           -v /var/run/wolf/wolf.sock:/var/run/wolf/wolf.sock:rw \
           -v ./wolf.conf:/etc/nginx/conf.d/wolf.conf:ro \
           nginx
You can now access the API via http://localhost:8080, ex:

curl localhost:8080/api/v1/openapi-schema
API Reference

Open SearchSearch
Keyboard Shortcut:CTRL⌃ k





































Open API Client
Powered by Scalar
v0.1
OAS 3.1.0
Wolf API

Download OpenAPI Document

Download OpenAPI Document
API for the Wolf server

Server
Server:
http://localhost
Local development server

Client Libraries
Shell Curl
Get all apps​Copy link
This endpoint returns a list of all apps.

Responses

200
application/json
Request Example forget/api/v1/apps

Shell Curl

curl http://localhost/api/v1/apps

Test Request
(get /api/v1/apps)
Status:200

{
  "apps": [
    {
      "av1_gst_pipeline": "string",
      "h264_gst_pipeline": "string",
      "hevc_gst_pipeline": "string",
      "icon_png_path": "string",
      "id": "string",
      "opus_gst_pipeline": "string",
      "render_node": "string",
      "runner": {
        "run_cmd": "string",
        "type": "process"
      },
      "start_audio_server": true,
      "start_virtual_compositor": true,
      "support_hdr": true,
      "title": "string"
    }
  ],
  "success": true
}
Get paired clients​Copy link
This endpoint returns a list of all paired clients.

Responses

200
application/json
Request Example forget/api/v1/clients

Shell Curl

curl http://localhost/api/v1/clients

Test Request
(get /api/v1/clients)
Status:200

{
  "clients": [
    {
      "app_state_folder": "string",
      "client_id": "string",
      "settings": {
        "controllers_override": [
          "XBOX"
        ],
        "h_scroll_acceleration": 1,
        "mouse_acceleration": 1,
        "run_gid": 1,
        "run_uid": 1,
        "v_scroll_acceleration": 1
      }
    }
  ],
  "success": true
}
Subscribe to events​Copy link
This endpoint allows clients to subscribe to events using SSE

Request Example forget/api/v1/events

Shell Curl

curl http://localhost/api/v1/events

Test Request
(get /api/v1/events)
Return this OpenAPI schema as JSON​Copy link
Request Example forget/api/v1/openapi-schema

Shell Curl

curl http://localhost/api/v1/openapi-schema

Test Request
(get /api/v1/openapi-schema)
Get pending pair requests​Copy link
This endpoint returns a list of Moonlight clients that are currently waiting to be paired.

Responses

200
application/json
Request Example forget/api/v1/pair/pending

Shell Curl

curl http://localhost/api/v1/pair/pending

Test Request
(get /api/v1/pair/pending)
Status:200

{
  "requests": [
    {
      "client_ip": "string",
      "pair_secret": "string"
    }
  ],
  "success": true
}
Get all stream sessions​Copy link
This endpoint returns a list of all active stream sessions.

Responses

200
application/json
Request Example forget/api/v1/sessions

Shell Curl

curl http://localhost/api/v1/sessions

Test Request
(get /api/v1/sessions)
Status:200

{
  "sessions": [
    {
      "app_id": "string",
      "audio_channel_count": 1,
      "client_id": "string",
      "client_ip": "string",
      "client_settings": {
        "controllers_override": [
          "XBOX"
        ],
        "h_scroll_acceleration": 1,
        "mouse_acceleration": 1,
        "run_gid": 1,
        "run_uid": 1,
        "v_scroll_acceleration": 1
      },
      "video_height": 1,
      "video_refresh_rate": 1,
      "video_width": 1
    }
  ],
  "success": true
}
Add an app​Copy link
Body
required
application/json
av1_gst_pipelineCopy link to av1_gst_pipeline
Type:string
required
h264_gst_pipelineCopy link to h264_gst_pipeline
Type:string
required
hevc_gst_pipelineCopy link to hevc_gst_pipeline
Type:string
required
idCopy link to id
Type:string
required
opus_gst_pipelineCopy link to opus_gst_pipeline
Type:string
required
render_nodeCopy link to render_node
Type:string
required
runnerCopy link to runner
required

Any of
wolf__config__AppCMD__tagged
run_cmd
Type:string
required
type
enum
const:
process
required
process
start_audio_serverCopy link to start_audio_server
Type:boolean
required
start_virtual_compositorCopy link to start_virtual_compositor
Type:boolean
required
support_hdrCopy link to support_hdr
Type:boolean
required
titleCopy link to title
Type:string
required
icon_png_pathCopy link to icon_png_path
Type:string
nullable
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/apps/add

Shell Curl

curl http://localhost/api/v1/apps/add \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "av1_gst_pipeline": "",
  "h264_gst_pipeline": "",
  "hevc_gst_pipeline": "",
  "icon_png_path": "",
  "id": "",
  "opus_gst_pipeline": "",
  "render_node": "",
  "runner": {
    "run_cmd": "string",
    "type": "process"
  },
  "start_audio_server": true,
  "start_virtual_compositor": true,
  "support_hdr": true,
  "title": ""
}'

Test Request
(post /api/v1/apps/add)
Status:200
Status:500

{
  "success": true
}
Remove an app​Copy link
Body
required
application/json
idCopy link to id
Type:string
required
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/apps/delete

Shell Curl

curl http://localhost/api/v1/apps/delete \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "id": ""
}'

Test Request
(post /api/v1/apps/delete)
Status:200
Status:500

{
  "success": true
}
Update client settings​Copy link
Update a client's settings including app state folder and client-specific settings

Body
required
application/json
app_state_folderCopy link to app_state_folder
Type:string
nullable
required
New app state folder path (optional)

client_idCopy link to client_id
Type:string
required
The client ID to identify the client (derived from certificate)

settingsCopy link to settings
Type:object
nullable
required
Client settings to update (only specified fields will be updated)

Show Child Attributesfor settings
Responses

200
application/json

400
application/json

404
application/json

500
application/json
Request Example forpost/api/v1/clients/settings

Shell Curl

curl http://localhost/api/v1/clients/settings \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "app_state_folder": "",
  "client_id": "",
  "settings": {
    "controllers_override": [
      "XBOX"
    ],
    "h_scroll_acceleration": 1,
    "mouse_acceleration": 1,
    "run_gid": 1,
    "run_uid": 1,
    "v_scroll_acceleration": 1
  }
}'

Test Request
(post /api/v1/clients/settings)
Status:200
Status:400
Status:404
Status:500

{
  "success": true
}
Pair a client​Copy link
Body
required
application/json
pair_secretCopy link to pair_secret
Type:string
required
pinCopy link to pin
Type:string
required
The PIN created by the remote Moonlight client

Responses

200
application/json

500
application/json
Request Example forpost/api/v1/pair/client

Shell Curl

curl http://localhost/api/v1/pair/client \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "pair_secret": "",
  "pin": ""
}'

Test Request
(post /api/v1/pair/client)
Status:200
Status:500

{
  "success": true
}
Start a runner in a given session​Copy link
Body
required
application/json
runnerCopy link to runner
required

Any of
wolf__config__AppCMD__tagged
run_cmd
Type:string
required
type
enum
const:
process
required
process
session_idCopy link to session_id
Type:string
required
stop_stream_when_overCopy link to stop_stream_when_over
Type:boolean
required
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/runners/start

Shell Curl

curl http://localhost/api/v1/runners/start \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "runner": {
    "run_cmd": "string",
    "type": "process"
  },
  "session_id": "",
  "stop_stream_when_over": true
}'

Test Request
(post /api/v1/runners/start)
Status:200
Status:500

{
  "success": true
}
Create a new stream session​Copy link
Body
required
application/json
app_idCopy link to app_id
Type:string
required
audio_channel_countCopy link to audio_channel_count
Type:integer
required
Integer numbers.

client_idCopy link to client_id
Type:string
required
client_ipCopy link to client_ip
Type:string
required
client_settingsCopy link to client_settings
Type:object
required
Show Child Attributesfor client_settings
video_heightCopy link to video_height
Type:integer
required
Integer numbers.

video_refresh_rateCopy link to video_refresh_rate
Type:integer
required
Integer numbers.

video_widthCopy link to video_width
Type:integer
required
Integer numbers.

Responses

200
application/json

500
application/json
Request Example forpost/api/v1/sessions/add

Shell Curl

curl http://localhost/api/v1/sessions/add \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "app_id": "",
  "audio_channel_count": 1,
  "client_id": "",
  "client_ip": "",
  "client_settings": {
    "controllers_override": [
      "XBOX"
    ],
    "h_scroll_acceleration": 1,
    "mouse_acceleration": 1,
    "run_gid": 1,
    "run_uid": 1,
    "v_scroll_acceleration": 1
  },
  "video_height": 1,
  "video_refresh_rate": 1,
  "video_width": 1
}'

Test Request
(post /api/v1/sessions/add)
Status:200
Status:500

{
  "session_id": "string",
  "success": true
}
Handle input for a stream session​Copy link
Body
required
application/json
input_packet_hexCopy link to input_packet_hex
Type:string
required
A HEX encoded Moonlight input packet, for the full format see: games-on-whales.github.io/wolf/stable/protocols/input-data.html

session_idCopy link to session_id
Type:string
required
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/sessions/input

Shell Curl

curl http://localhost/api/v1/sessions/input \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "input_packet_hex": "",
  "session_id": ""
}'

Test Request
(post /api/v1/sessions/input)
Status:200
Status:500

{
  "success": true
}
Pause a stream session​Copy link
Body
required
application/json
session_idCopy link to session_id
Type:string
required
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/sessions/pause

Shell Curl

curl http://localhost/api/v1/sessions/pause \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "session_id": ""
}'

Test Request
(post /api/v1/sessions/pause)
Status:200
Status:500

{
  "success": true
}
Start a stream session​Copy link
Body
required
application/json
audio_sessionCopy link to audio_session
Type:object
required
Show Child Attributesfor audio_session
session_idCopy link to session_id
Type:string
required
video_sessionCopy link to video_session
Type:object
required
Show Child Attributesfor video_session
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/sessions/start

Shell Curl

curl http://localhost/api/v1/sessions/start \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "audio_session": {
    "aes_iv": "",
    "aes_key": "",
    "audio_mode": {
      "bitrate": 1,
      "channels": 1,
      "coupled_streams": 1,
      "sample_rate": 1,
      "speakers": [
        "FRONT_LEFT"
      ],
      "streams": 1
    },
    "client_ip": "",
    "encrypt_audio": true,
    "gst_pipeline": "",
    "packet_duration": 1,
    "port": 1,
    "session_id": 1,
    "wait_for_ping": true
  },
  "session_id": "",
  "video_session": {
    "bitrate_kbps": 1,
    "client_ip": "",
    "color_range": "JPEG",
    "color_space": "BT601",
    "display_mode": {
      "height": 1,
      "refreshRate": 1,
      "width": 1
    },
    "fec_percentage": 1,
    "frames_with_invalid_ref_threshold": 1,
    "gst_pipeline": "",
    "min_required_fec_packets": 1,
    "packet_size": 1,
    "port": 1,
    "session_id": 1,
    "slices_per_frame": 1,
    "timeout_ms": 1,
    "wait_for_ping": true
  }
}'

Test Request
(post /api/v1/sessions/start)
Status:200
Status:500

{
  "success": true
}
Stop a stream session​Copy link
Body
required
application/json
session_idCopy link to session_id
Type:string
required
Responses

200
application/json

500
application/json
Request Example forpost/api/v1/sessions/stop

Shell Curl

curl http://localhost/api/v1/sessions/stop \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "session_id": ""
}'

Test Request
(post /api/v1/sessions/stop)
Status:200
Status:500

{
  "success": true
}
Unpair a client​Copy link
Body
required
application/json
client_idCopy link to client_id
Type:string
required
The client ID to unpair

Responses

200
application/json

500
application/json
Request Example forpost/api/v1/unpair/client

Shell Curl

curl http://localhost/api/v1/unpair/client \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{
  "client_id": ""
}'

Test Request
(post /api/v1/unpair/client)
Status:200
Status:500

{
  "success": true
}
Models

rfl__Reflector_wolf__core__events__App___ReflType​Copy link

rfl__Reflector_wolf__core__events__StreamSession___ReflType​Copy link

wolf__api__AppDeleteRequest​Copy link

wolf__api__AppListResponse​Copy link

wolf__api__GenericErrorResponse​Copy link

wolf__api__GenericSuccessResponse​Copy link

wolf__api__PairRequest​Copy link

wolf__api__PairedClient​Copy link

wolf__api__PairedClientsResponse​Copy link

wolf__api__PartialClientSettings​Copy link
Show More

Copyright © ABeltramo and GOW contributors.

Except where otherwise noted, docs are licensed under the Creative Commons Attribution-ShareAlike 4.0 International (CC BY-SA 4.0).
