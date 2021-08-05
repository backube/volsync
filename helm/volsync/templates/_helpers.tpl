{{/*
Expand the name of the chart.
*/}}
{{- define "volsync.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "volsync.fullname" -}}
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
{{- define "volsync.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "volsync.labels" -}}
helm.sh/chart: {{ include "volsync.chart" . }}
{{ include "volsync.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "volsync.selectorLabels" -}}
app.kubernetes.io/name: {{ include "volsync.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "volsync.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "volsync.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Determine the container image to use
Usage: {{- include "container-image" (list $ .Values.image) }}
This horrible hack from: https://blog.flant.com/advanced-helm-templating/
*/}}
{{- define "container-image" -}}
{{- $ := index . 0 }}
{{- with index . 1 }}
{{- if .image -}}
{{ .image }}
{{- else -}}
{{ .repository }}:{{ .tag | default $.Chart.AppVersion }}
{{- end -}}
{{- end -}}
{{- end }}
