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
{{- if .Values.kubeApiToken.enabled }}
- name: kube-api
  mountPath: /var/run/secrets/kubernetes.io/serviceaccount
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
{{- if .Values.kubeApiToken.enabled }}
- name: kube-api
  projected:
    sources:
      - serviceAccountToken:
          expirationSeconds: 3600
          path: token
      - configMap:
          name: kube-root-ca.crt
          items: [{ key: ca.crt, path: ca.crt }]
      - downwardAPI:
          items: [{ path: namespace, fieldRef: { fieldPath: metadata.namespace } }]
{{- end }}
{{- end }}

{{/* The image reference (repo:tag@digest), after the Kyverno baseline checks
     that apply to every workload kind. Shared by the Deployment and CronJob. */}}
{{- define "app.image" -}}
{{- $repo := required "image.repository is required" .Values.image.repository -}}
{{- $tag := required "image.tag is required" .Values.image.tag | toString -}}
{{- if eq $tag "latest" }}{{ fail "image.tag must not be \"latest\" (Kyverno disallow-latest-tag)" }}{{ end -}}
{{- $digest := .Values.image.digest | default "" | toString -}}
{{- if and $digest (not (regexMatch "^sha256:[a-f0-9]{64}$" $digest)) }}{{ fail "image.digest must look like sha256:<64 hex chars>" }}{{ end -}}
{{- $image := printf "%s:%s" $repo $tag -}}
{{- /* Kyverno baseline (infra/k8s/kyverno/policies) checked at render time, so a
       bad values file fails in PR CI instead of at admission during sync. */ -}}
{{- if lt (int .Values.podSecurityContext.runAsUser) 1 }}{{ fail "podSecurityContext.runAsUser must be a non-root UID (> 0) - Kyverno require-non-root" }}{{ end -}}
{{- $res := .Values.resources | default dict -}}
{{- range $kind := list "requests" "limits" }}
{{- $r := get $res $kind | default dict -}}
{{- range $k := list "cpu" "memory" }}
{{- if not (get $r $k) }}{{ fail (printf "resources.%s.%s is required - Kyverno require-resource-limits" $kind $k) }}{{ end -}}
{{- end }}
{{- end -}}
{{- if $digest }}{{ $image = printf "%s@%s" $image $digest }}{{ end -}}
{{- $image -}}
{{- end }}
