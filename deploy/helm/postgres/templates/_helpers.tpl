{{/* The Service, StatefulSet and Secret share this name, so services reach PostgreSQL at <release>-postgres. */}}
{{- define "postgres.name" -}}
{{ .Release.Name }}-postgres
{{- end -}}
