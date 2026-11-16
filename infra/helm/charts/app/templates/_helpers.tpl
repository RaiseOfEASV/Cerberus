{{/* Release names are set explicitly per app in each ArgoCD Application. */}}
{{- define "app.fullname" -}}
{{ .Release.Name }}
{{- end }}

{{- define "app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "app.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "app.labels" -}}
{{ include "app.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{- define "app.serviceAccountName" -}}
{{ .Values.serviceAccount.name | default (include "app.fullname" .) }}
{{- end }}

{{/* Kyverno baseline container securityContext, shared by app and init. */}}
{{- define "app.containerSecurityContext" -}}
runAsNonRoot: true
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
seccompProfile:
  type: RuntimeDefault
{{- end }}

{{- define "app.volumeMounts" -}}
{{- range $i, $p := .Values.writablePaths }}
- name: writable-{{ $i }}
  mountPath: {{ $p }}
{{- end }}
{{- if .Values.vaultToken.enabled }}
- name: vault-token
  mountPath: {{ .Values.vaultToken.path }}
  readOnly: true
{{- end }}
{{- if .Values.caConfigMap }}
- name: cerberus-ca
  mountPath: /etc/cerberus
  readOnly: true
{{- end }}
{{- end }}

{{- define "app.volumes" -}}
{{- range $i, $p := .Values.writablePaths }}
- name: writable-{{ $i }}
  emptyDir: {}
{{- end }}
{{- if .Values.vaultToken.enabled }}
- name: vault-token
  projected:
    sources:
      - serviceAccountToken:
          audience: {{ .Values.vaultToken.audience }}
          expirationSeconds: 600
          path: token
{{- end }}
{{- if .Values.caConfigMap }}
- name: cerberus-ca
  configMap:
    name: {{ .Values.caConfigMap }}
{{- end }}
{{- end }}
