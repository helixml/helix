{{/*
Expand the name of the chart.
*/}}
{{- define "helix-sandbox.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "helix-sandbox.fullname" -}}
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
{{- define "helix-sandbox.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "helix-sandbox.labels" -}}
helm.sh/chart: {{ include "helix-sandbox.chart" . }}
{{ include "helix-sandbox.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "helix-sandbox.selectorLabels" -}}
app.kubernetes.io/name: {{ include "helix-sandbox.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "helix-sandbox.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "helix-sandbox.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Determine the GPU vendor from values
*/}}
{{- define "helix-sandbox.gpuVendor" -}}
{{- .Values.gpu.vendor | default "nvidia" }}
{{- end }}

{{/*
Determine if software rendering is enabled (no GPU)
*/}}
{{- define "helix-sandbox.isSoftwareRendering" -}}
{{- if eq (include "helix-sandbox.gpuVendor" .) "none" }}true{{- else }}false{{- end }}
{{- end }}

{{/*
Get the render node based on GPU vendor
*/}}
{{- define "helix-sandbox.renderNode" -}}
{{- if .Values.gpu.renderNode -}}
{{- .Values.gpu.renderNode -}}
{{- else if eq (include "helix-sandbox.gpuVendor" .) "none" -}}
software
{{- else -}}
/dev/dri/renderD128
{{- end -}}
{{- end }}

{{/*
Determine runtime class name based on GPU vendor
*/}}
{{- define "helix-sandbox.runtimeClassName" -}}
{{- $vendor := include "helix-sandbox.gpuVendor" . }}
{{- if eq $vendor "nvidia" }}
{{- .Values.gpu.nvidia.runtimeClassName | default "" }}
{{- else if eq $vendor "amd" }}
{{- .Values.gpu.amd.runtimeClassName | default "" }}
{{- else if eq $vendor "intel" }}
{{- .Values.gpu.intel.runtimeClassName | default "" }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}
