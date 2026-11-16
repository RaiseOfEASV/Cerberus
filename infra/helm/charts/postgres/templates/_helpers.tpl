{{- define "pg.name" -}}{{ .Release.Name }}{{- end }}
{{- define "pg.labels" -}}
app.kubernetes.io/name: {{ include "pg.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: database
{{- end }}
{{- define "pg.selector" -}}
app.kubernetes.io/name: {{ include "pg.name" . }}
app.kubernetes.io/component: database
{{- end }}
{{- define "pg.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities: { drop: ["ALL"] }
{{- end }}
