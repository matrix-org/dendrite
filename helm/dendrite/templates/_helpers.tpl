{{- define "validate.config" }}
{{- if not .Values.signing_key.create -}}
{{-  fail "You must create a signing key for configuration.signing_key. (see https://github.com/matrix-org/dendrite/blob/master/docs/INSTALL.md#server-key-generation)" -}}
{{- end -}}
{{- if not (or .Values.dendrite_config.global.database.host .Values.postgresql.enabled) -}}
{{-  fail "Database server must be set." -}}
{{- end -}}
{{- if not (or .Values.dendrite_config.global.database.user .Values.postgresql.enabled) -}}
{{-  fail "Database user must be set." -}}
{{- end -}}
{{- if not (or .Values.dendrite_config.global.database.password .Values.postgresql.enabled) -}}
{{-  fail "Database password must be set." -}}
{{- end -}}
{{- end -}}


{{- define "image.name" -}}
{{- with .Values.image -}}
image: {{ .repository }}:{{ .tag | default (printf "v%s" $.Chart.AppVersion) }}
imagePullPolicy: {{ .pullPolicy }}
{{- end -}}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "dendrite.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dendrite.fullname" -}}
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
{{- define "dendrite.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "dendrite.labels" -}}
helm.sh/chart: {{ include "dendrite.chart" . }}
{{ include "dendrite.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "dendrite.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dendrite.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}