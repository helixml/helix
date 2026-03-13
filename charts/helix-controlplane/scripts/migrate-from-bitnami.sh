#!/bin/bash
set -euo pipefail

# Migration script: Bitnami PostgreSQL → Official PostgreSQL
#
# For customers upgrading helix-controlplane chart from <0.4.0 to >=0.4.0
# who want to preserve their database.
#
# The chart moved from the Bitnami PostgreSQL subchart (StatefulSet) to
# an official postgres:17-alpine Deployment. PVC names, service names,
# and data directory layouts are incompatible, so a dump/restore is required.
#
# Usage:
#   ./migrate-from-bitnami.sh --release <name> [--namespace <ns>]
#
# Prerequisites:
#   - kubectl configured with access to the cluster
#   - helm v3
#   - The old chart version still running (pre-0.4.0)
#
# What this script does:
#   Phase 1 (pre-upgrade):  pg_dump from the old Bitnami postgres pod
#   Phase 2 (user action):  prompts you to run helm upgrade
#   Phase 3 (post-upgrade): pg_restore into the new official postgres pod

usage() {
  echo "Usage: $0 --release <name> [--namespace <ns>]"
  echo ""
  echo "Options:"
  echo "  --release    Helm release name (required)"
  echo "  --namespace  Kubernetes namespace (default: default)"
  echo ""
  echo "Example: $0 --release my-helix --namespace production"
  exit 1
}

RELEASE=""
NAMESPACE="default"

while [ $# -gt 0 ]; do
  case "$1" in
    --release)    RELEASE="$2"; shift 2 ;;
    --namespace)  NAMESPACE="$2"; shift 2 ;;
    -h|--help)    usage ;;
    *)            echo "Unknown option: $1"; usage ;;
  esac
done

if [ -z "$RELEASE" ]; then
  echo "ERROR: --release is required"
  echo ""
  usage
fi

DUMP_FILE="/tmp/helix-pg-migration-${RELEASE}.sql"

NS_FLAG="-n ${NAMESPACE}"

echo "=== Helix PostgreSQL Migration: Bitnami → Official ==="
echo "Release:   ${RELEASE}"
echo "Namespace: ${NAMESPACE}"
echo ""

# -------------------------------------------------------
# Phase 1: Dump from old Bitnami postgres
# -------------------------------------------------------
echo "--- Phase 1: Dump existing database ---"

# Bitnami subchart creates a StatefulSet named {release}-postgresql
OLD_POD="${RELEASE}-postgresql-0"

echo "Checking old Bitnami pod ${OLD_POD}..."
if ! kubectl get pod ${NS_FLAG} "${OLD_POD}" &>/dev/null; then
  echo "ERROR: Pod ${OLD_POD} not found in namespace ${NAMESPACE}."
  echo "Make sure the old chart version is still running before migrating."
  exit 1
fi

# Get credentials from the old controlplane deployment
echo "Reading database credentials from controlplane deployment..."
POSTGRES_USER=$(kubectl get deploy ${NS_FLAG} "${RELEASE}-helix-controlplane" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_USER")].value}' 2>/dev/null || \
  kubectl get deploy ${NS_FLAG} "${RELEASE}" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_USER")].value}' 2>/dev/null || \
  echo "helix")
POSTGRES_DB=$(kubectl get deploy ${NS_FLAG} "${RELEASE}-helix-controlplane" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_DATABASE")].value}' 2>/dev/null || \
  kubectl get deploy ${NS_FLAG} "${RELEASE}" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_DATABASE")].value}' 2>/dev/null || \
  echo "helix")

# Get the password from the Bitnami secret ({release}-postgresql)
BITNAMI_SECRET="${RELEASE}-postgresql"
echo "Reading password from secret ${BITNAMI_SECRET}..."
POSTGRES_PASSWORD=$(kubectl get secret ${NS_FLAG} "${BITNAMI_SECRET}" -o jsonpath='{.data.password}' 2>/dev/null | base64 -d || \
  echo "")
if [ -z "${POSTGRES_PASSWORD}" ]; then
  echo "WARNING: Could not read password from secret ${BITNAMI_SECRET}."
  echo "Trying to read from controlplane deployment env..."
  POSTGRES_PASSWORD=$(kubectl get deploy ${NS_FLAG} "${RELEASE}-helix-controlplane" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_PASSWORD")].value}' 2>/dev/null || \
    echo "")
fi
if [ -z "${POSTGRES_PASSWORD}" ]; then
  echo "ERROR: Could not determine PostgreSQL password."
  exit 1
fi

echo "User: ${POSTGRES_USER}, Database: ${POSTGRES_DB}"

echo "Running pg_dump from ${OLD_POD} (database: ${POSTGRES_DB})..."
kubectl exec ${NS_FLAG} "${OLD_POD}" -- env PGPASSWORD="${POSTGRES_PASSWORD}" pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" > "${DUMP_FILE}"

DUMP_SIZE=$(wc -c < "${DUMP_FILE}" | tr -d ' ')
echo "Dump complete: ${DUMP_FILE} (${DUMP_SIZE} bytes)"

if [ "${DUMP_SIZE}" -lt 100 ]; then
  echo "WARNING: Dump file is suspiciously small. Check for errors above."
  echo "Contents:"
  cat "${DUMP_FILE}"
  exit 1
fi

# -------------------------------------------------------
# Phase 2: User upgrades the chart
# -------------------------------------------------------
echo ""
echo "--- Phase 2: Upgrade the Helm chart ---"
echo ""
echo "The database has been dumped to: ${DUMP_FILE}"
echo ""
echo "Now upgrade the chart to >=0.4.0. For example:"
echo ""
echo "  helm upgrade ${RELEASE} helix/helix-controlplane \\"
echo "    -n ${NAMESPACE} \\"
echo "    -f your-values.yaml \\"
echo "    --version 0.4.0"
echo ""
echo "After the upgrade completes and the new postgres pod is running,"
echo "press Enter to continue with the restore..."
read -r

# -------------------------------------------------------
# Phase 3: Restore into new official postgres
# -------------------------------------------------------
echo "--- Phase 3: Restore database ---"

# The new deployment creates a pod with label app.kubernetes.io/component=postgres
echo "Waiting for new postgres pod to be ready..."
kubectl wait ${NS_FLAG} --for=condition=ready pod \
  -l "app.kubernetes.io/component=postgres,app.kubernetes.io/instance=${RELEASE}" \
  --timeout=120s

NEW_POD=$(kubectl get pod ${NS_FLAG} \
  -l "app.kubernetes.io/component=postgres,app.kubernetes.io/instance=${RELEASE}" \
  -o jsonpath='{.items[0].metadata.name}')

echo "New postgres pod: ${NEW_POD}"

echo "Copying dump file to new pod..."
kubectl cp ${NS_FLAG} "${DUMP_FILE}" "${NEW_POD}:/tmp/helix-migration.sql"

# Get the new postgres password from the new deployment's env
echo "Reading new postgres credentials..."
NEW_PASSWORD=$(kubectl get deploy ${NS_FLAG} \
  -l "app.kubernetes.io/component=postgres,app.kubernetes.io/instance=${RELEASE}" \
  -o jsonpath='{.items[0].spec.template.spec.containers[0].env[?(@.name=="POSTGRES_PASSWORD")].value}' 2>/dev/null || \
  echo "${POSTGRES_PASSWORD}")

echo "Restoring database..."
kubectl exec ${NS_FLAG} "${NEW_POD}" -- \
  env PGPASSWORD="${NEW_PASSWORD}" psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -f /tmp/helix-migration.sql

echo "Cleaning up dump file from pod..."
kubectl exec ${NS_FLAG} "${NEW_POD}" -- rm -f /tmp/helix-migration.sql

# -------------------------------------------------------
# Phase 4: Verify
# -------------------------------------------------------
echo ""
echo "--- Phase 4: Verify ---"

TABLE_COUNT=$(kubectl exec ${NS_FLAG} "${NEW_POD}" -- \
  env PGPASSWORD="${NEW_PASSWORD}" psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -t -c \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" | tr -d ' ')

echo "Tables in public schema: ${TABLE_COUNT}"

if [ "${TABLE_COUNT}" -gt 0 ]; then
  echo ""
  echo "=== Migration successful ==="
  echo ""
  echo "Restart the controlplane to reconnect:"
  echo "  kubectl rollout restart deploy ${NS_FLAG} -l app.kubernetes.io/component=controlplane,app.kubernetes.io/instance=${RELEASE}"
  echo ""
  echo "Once verified, you can delete the old Bitnami PVC:"
  echo "  kubectl delete pvc ${NS_FLAG} data-${RELEASE}-postgresql-0"
  echo ""
  echo "Local dump file preserved at: ${DUMP_FILE}"
else
  echo ""
  echo "WARNING: No tables found after restore. Check the output above for errors."
  echo "The dump file is preserved at: ${DUMP_FILE}"
  exit 1
fi
