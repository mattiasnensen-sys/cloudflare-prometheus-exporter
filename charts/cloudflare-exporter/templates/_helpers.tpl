{{- define "cloudflare-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "cloudflare-exporter.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "cloudflare-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "cloudflare-exporter.labels" -}}
helm.sh/chart: {{ include "cloudflare-exporter.chart" . }}
{{ include "cloudflare-exporter.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "cloudflare-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cloudflare-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "cloudflare-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "cloudflare-exporter.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "cloudflare-exporter.secretName" -}}
{{- if .Values.secret.create -}}
{{- default (printf "%s-credentials" (include "cloudflare-exporter.fullname" .)) .Values.secret.name -}}
{{- else -}}
{{- .Values.cloudflare.existingSecret.name -}}
{{- end -}}
{{- end -}}
