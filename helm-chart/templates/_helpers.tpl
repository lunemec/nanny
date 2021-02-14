{{/*
Expand the name of the chart.
*/}}
{{- define "nanny.name" -}}
{{- default .Chart.Name .Values.nanny.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "nanny.fullname" -}}
{{- if .Values.nanny.fullnameOverride }}
{{- .Values.nanny.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nanny.nameOverride }}
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
{{- define "nanny.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nanny.labels" -}}
helm.sh/chart: {{ include "nanny.chart" . }}
{{ include "nanny.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nanny.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nanny.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "nanny.serviceAccountName" -}}
{{- if .Values.nanny.serviceAccount.create }}
{{- default (include "nanny.fullname" .) .Values.nanny.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.nanny.serviceAccount.name }}
{{- end }}
{{- end }}
