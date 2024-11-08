#!/bin/bash

HELIX_URL="${HELIX_URL:-http://localhost:8080}"

if [ -z "${HELIX_API_KEY:-}" ]; then
    echo "HELIX_API_KEY is not set. Please set it before proceeding."
    exit 1
fi

status_code=$(curl -o /dev/null -s -w "%{http_code}\n" -H "Authorization: Bearer $HELIX_API_KEY" "$HELIX_URL/api/v1/dashboard")

if [ "$status_code" -ne 200 ]; then
    echo "Failed to connect to HELIX_URL. Status code: $status_code"
    exit 1
fi

# A for loop that repeats every 1 second
while true; do
dashboard_data=$(curl -s -H "Authorization: Bearer $HELIX_API_KEY" "$HELIX_URL/api/v1/dashboard")
runner_ids=$(echo $dashboard_data | jq -r '.desired_slots | map(.id) | sort | @csv')

clear printf '\e[3J'
for id in $(echo $runner_ids | tr "," "\n" | tr -d '"'); do
desired_slot_ids=$(echo $dashboard_data | jq -r --arg ID "$id" '.desired_slots | .[] | select(.id == $ID) | .data | map(.id) | sort | @csv')
actual_slot_ids=$(echo $dashboard_data | jq -r --arg ID "$id" '.runners | .[] | select(.id == $ID) | .slots | map(.id) | sort | @csv')
printf '==== %s ====\n' $id;
printf 'Desired:\n';
for slot_id in $(echo $desired_slot_ids | tr "," "\n" | tr -d '"'); do
    model_name=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.desired_slots | .[] | select(.id == $ID) | .data | .[] | select(.id == $SLOT_ID) | .attributes.model')
    content=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.desired_slots | .[] | select(.id == $ID) | .data | .[] | select(.id == $SLOT_ID) | .attributes.workload.LLMInferenceRequest.Request.messages[-1].content')
    request_id=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.desired_slots | .[] | select(.id == $ID) | .data | .[] | select(.id == $SLOT_ID) | .attributes.workload.LLMInferenceRequest.RequestID')
    if [[ $actual_slot_ids == *"$slot_id"* ]]; then
        printf '  - %s : %s\n' $slot_id $model_name;
        printf '    ↳ %s: %s\n' $request_id "$content";
    else
        printf '  - \e[31m%s\e[0m : %s\n' $slot_id  $model_name;
        printf '    ↳ %s: %s\n' $request_id "$content";
    fi
done
printf 'Actual:\n';
for slot_id in $(echo $actual_slot_ids | tr "," "\n" | tr -d '"'); do
    model_name=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.runners | .[] | select(.id == $ID) | .slots | .[] | select(.id == $SLOT_ID) | .attributes.current_workload.LLMInferenceRequest.Request.model')
    content=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.runners | .[] | select(.id == $ID) | .slots | .[] | select(.id == $SLOT_ID) | .attributes.current_workload.LLMInferenceRequest.Request.messages[-1].content')
    request_id=$(echo $dashboard_data | jq -r --arg ID "$id" --arg SLOT_ID "$slot_id" '.runners | .[] | select(.id == $ID) | .slots | .[] | select(.id == $SLOT_ID) | .attributes.current_workload.LLMInferenceRequest.RequestID')
    if [[ $desired_slot_ids == *"$slot_id"* ]]; then
        printf '  - %s : %s\n' $slot_id $model_name;
        printf '    ↳ %s: %s\n' $request_id "$content";
    else
        printf '  - \e[31m%s\e[0m : %s\n' $slot_id  $model_name;
        printf '    ↳ %s: %s\n' $request_id "$content";
    fi
done
done

sleep 1
done




