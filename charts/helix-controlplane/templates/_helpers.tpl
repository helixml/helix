{{/*
Expand the name of the chart.
*/}}
{{- define "helix-controlplane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "helix-controlplane.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "helix-controlplane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "helix-controlplane.labels" -}}
helm.sh/chart: {{ include "helix-controlplane.chart" . }}
{{ include "helix-controlplane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "helix-controlplane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "helix-controlplane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "helix-controlplane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "helix-controlplane.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Renders a value that contains a template.
Replaces Bitnami common.tplvalues.render.
Usage: {{ include "helix-controlplane.tplvalues.render" (dict "value" .Values.some.value "context" $) }}
*/}}
{{- define "helix-controlplane.tplvalues.render" -}}
{{- if typeIs "string" .value }}
{{- tpl .value .context }}
{{- else }}
{{- tpl (.value | toYaml) .context }}
{{- end }}
{{- end -}}

{{/*
Return the appropriate storageClass for a PVC.
Replaces Bitnami common.storage.class.
Usage: {{ include "helix-controlplane.storage.class" (dict "persistence" .Values.some.persistence "global" .Values.global) }}
*/}}
{{- define "helix-controlplane.storage.class" -}}
{{- $storageClass := "" -}}
{{- if .global -}}
{{- if .global.storageClass -}}
{{- $storageClass = .global.storageClass -}}
{{- end -}}
{{- end -}}
{{- if .persistence.storageClass -}}
{{- $storageClass = .persistence.storageClass -}}
{{- end -}}
{{- if $storageClass -}}
{{- if (eq "-" $storageClass) }}
storageClassName: ""
{{- else }}
storageClassName: {{ $storageClass | quote }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
PostgreSQL image reference, honoring global.imageRegistry.
Usage: image: {{ include "helix-controlplane.postgres-image" . | quote }}
*/}}
{{- define "helix-controlplane.postgres-image" -}}
{{- if .Values.global.imageRegistry -}}
{{ .Values.global.imageRegistry }}/{{ .Values.postgresql.image.repository }}:{{ .Values.postgresql.image.tag }}
{{- else -}}
{{ .Values.postgresql.image.repository }}:{{ .Values.postgresql.image.tag }}
{{- end -}}
{{- end -}}

{{/*
PostgreSQL connection environment variables.
Used by both the init container and main controlplane container.
*/}}
{{- define "helix-controlplane.postgres-env" -}}
{{- if .Values.postgresql.enabled }}
- name: POSTGRES_HOST
  value: {{ include "helix-controlplane.fullname" . }}-postgres
- name: POSTGRES_PORT
  value: "5432"
{{- if .Values.postgresql.auth.existingSecret }}
- name: POSTGRES_USER
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.auth.existingSecret }}
      key: {{ .Values.postgresql.auth.usernameKey | default "username" }}
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.auth.existingSecret }}
      key: {{ .Values.postgresql.auth.passwordKey | default "password" }}
- name: POSTGRES_DATABASE
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.auth.existingSecret }}
      key: {{ .Values.postgresql.auth.databaseKey | default "database" }}
{{- else }}
- name: POSTGRES_USER
  value: {{ .Values.postgresql.auth.username }}
- name: POSTGRES_PASSWORD
  value: {{ .Values.postgresql.auth.password }}
- name: POSTGRES_DATABASE
  value: {{ .Values.postgresql.auth.database }}
{{- end }}
{{- else if .Values.postgresql.external.existingSecret }}
- name: POSTGRES_HOST
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.external.existingSecret }}
      key: {{ .Values.postgresql.external.existingSecretHostKey | default "host" }}
- name: POSTGRES_PORT
  value: {{ .Values.postgresql.external.port | default 5432 | quote }}
- name: POSTGRES_USER
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.external.existingSecret }}
      key: {{ .Values.postgresql.external.existingSecretUserKey | default "user" }}
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.external.existingSecret }}
      key: {{ .Values.postgresql.external.existingSecretPasswordKey | default "password" }}
- name: POSTGRES_DATABASE
  valueFrom:
    secretKeyRef:
      name: {{ .Values.postgresql.external.existingSecret }}
      key: {{ .Values.postgresql.external.existingSecretDatabaseKey | default "database" }}
{{- else }}
- name: POSTGRES_HOST
  value: {{ .Values.postgresql.external.host }}
- name: POSTGRES_PORT
  value: {{ .Values.postgresql.external.port | default 5432 | quote }}
- name: POSTGRES_USER
  value: {{ .Values.postgresql.external.user }}
- name: POSTGRES_PASSWORD
  value: {{ .Values.postgresql.external.password }}
- name: POSTGRES_DATABASE
  value: {{ .Values.postgresql.external.database }}
{{- end }}
{{- end }}

{{/*
Crawler browser launcher token: chrome.launcherToken, else the value already
in the chart's Secret, else a new random one. Memoised in .Values so every
template renders the same value.
*/}}
{{- define "helix-controlplane.chrome-launcher-token" -}}
{{- if not .Values.chrome._launcherToken -}}
{{- $token := .Values.chrome.launcherToken -}}
{{- if not $token -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace (include "helix-controlplane.chrome-launcher-secret" .) -}}
{{- if and $existing $existing.data (hasKey $existing.data "token") -}}
{{- $token = index $existing.data "token" | b64dec -}}
{{- else -}}
{{- $token = randAlphaNum 48 -}}
{{- end -}}
{{- end -}}
{{- $_ := set .Values.chrome "_launcherToken" $token -}}
{{- end -}}
{{- .Values.chrome._launcherToken -}}
{{- end -}}

{{- define "helix-controlplane.chrome-launcher-secret" -}}
{{- .Values.chrome.launcherTokenExistingSecret | default (printf "%s-chrome-launcher" (include "helix-controlplane.fullname" .)) -}}
{{- end -}}

{{/*
Env var carrying the launcher token from its Secret. Usage:
{{ include "helix-controlplane.chrome-launcher-env" (dict "name" "CHROME_LAUNCHER_TOKEN" "root" .) }}
*/}}
{{- define "helix-controlplane.chrome-launcher-env" -}}
- name: {{ .name }}
  valueFrom:
    secretKeyRef:
      name: {{ include "helix-controlplane.chrome-launcher-secret" .root }}
      key: {{ if .root.Values.chrome.launcherTokenExistingSecret }}{{ .root.Values.chrome.launcherTokenExistingSecretKey | default "token" }}{{ else }}token{{ end }}
{{- end -}}

{{/*
Pod annotation that rolls the controlplane and chrome pods together when a
chart-managed launcher token changes.
*/}}
{{- define "helix-controlplane.chrome-launcher-checksum" -}}
{{- if and .Values.chrome.enabled (not .Values.chrome.launcherTokenExistingSecret) -}}
checksum/chrome-launcher-token: {{ include "helix-controlplane.chrome-launcher-token" . | sha256sum | quote }}
{{- end -}}
{{- end -}}
