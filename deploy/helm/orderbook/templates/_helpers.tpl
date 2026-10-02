{{- define "orderbook.image" -}}
{{ .root.Values.image.repository }}/{{ .service }}:{{ .root.Values.image.tag }}
{{- end -}}

{{/* PostgreSQL's password, from the postgres chart's Secret. */}}
{{- define "orderbook.dbPassword" -}}
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Release.Name }}-postgres
      key: password
{{- end -}}
