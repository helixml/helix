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
Runner token secret name.
Returns the secret name containing the runner token, whether user-provided or auto-generated.
*/}}
{{- define "helix-controlplane.runner-token-secret-name" -}}
{{- if .Values.controlplane.runnerTokenExistingSecret -}}
{{ .Values.controlplane.runnerTokenExistingSecret }}
{{- else -}}
{{ include "helix-controlplane.fullname" . }}-runner-token
{{- end -}}
{{- end }}

{{/*
Runner token secret key.
*/}}
{{- define "helix-controlplane.runner-token-secret-key" -}}
{{- if .Values.controlplane.runnerTokenExistingSecret -}}
{{ .Values.controlplane.runnerTokenExistingSecretKey | default "token" }}
{{- else -}}
token
{{- end -}}
{{- end }}

{{/*
PostgreSQL connection environment variables.
Used by both the init container and main controlplane container.
*/}}
{{- define "helix-controlplane.postgres-env" -}}
{{- if .Values.postgresql.enabled }}
- name: POSTGRES_HOST
  value: {{ .Release.Name }}-postgresql
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
