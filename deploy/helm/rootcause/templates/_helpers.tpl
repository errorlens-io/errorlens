{{- define "rootcause.fullname" -}}{{ .Release.Name }}-rootcause{{- end -}}
{{- define "rootcause.image" -}}{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}{{- end -}}
{{- define "rootcause.labels" -}}
app.kubernetes.io/name: rootcause
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{- define "rootcause.env" -}}
- name: ROOTCAUSE_PUBLIC_URL
  value: {{ .Values.publicURL | quote }}
- name: ROOTCAUSE_DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Values.database.existingSecret | default (printf "%s-secrets" (include "rootcause.fullname" .)) }}
      key: DATABASE_URL
{{- if or .Values.encryption.existingSecret .Values.encryption.key }}
- name: ROOTCAUSE_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.encryption.existingSecret | default (printf "%s-secrets" (include "rootcause.fullname" .)) }}
      key: ENCRYPTION_KEY
{{- end }}
{{- range $k, $v := .Values.env }}
- name: {{ $k }}
  value: {{ $v | quote }}
{{- end }}
{{- end -}}
