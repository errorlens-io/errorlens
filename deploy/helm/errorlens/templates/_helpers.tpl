{{- define "errorlens.fullname" -}}{{ .Release.Name }}-errorlens{{- end -}}
{{- define "errorlens.image" -}}{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}{{- end -}}
{{- define "errorlens.labels" -}}
app.kubernetes.io/name: errorlens
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{- define "errorlens.env" -}}
- name: ERRORLENS_PUBLIC_URL
  value: {{ .Values.publicURL | quote }}
- name: ERRORLENS_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.database.existingSecret | default (printf "%s-secrets" (include "errorlens.fullname" .)) }}
      key: DATABASE_URL
{{- if or .Values.encryption.existingSecret .Values.encryption.key }}
- name: ERRORLENS_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.encryption.existingSecret | default (printf "%s-secrets" (include "errorlens.fullname" .)) }}
      key: ENCRYPTION_KEY
{{- end }}
{{- range $k, $v := .Values.env }}
- name: {{ $k }}
  value: {{ $v | quote }}
{{- end }}
{{- end -}}
