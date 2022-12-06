{{- define "validate.config" }}
{{- if not .Values.configuration.signing_key.create -}}
{{-  fail "You must create a signing key for configuration.signing_key. (see https://github.com/matrix-org/dendrite/blob/master/docs/INSTALL.md#server-key-generation)" -}}
{{- end -}}
{{- if not (or .Values.configuration.database.host .Values.postgresql.enabled) -}}
{{-  fail "Database server must be set." -}}
{{- end -}}
{{- if not (or .Values.configuration.database.user .Values.postgresql.enabled) -}}
{{-  fail "Database user must be set." -}}
{{- end -}}
{{- if not (or .Values.configuration.database.password .Values.postgresql.enabled) -}}
{{-  fail "Database password must be set." -}}
{{- end -}}
{{- end -}}

{{- define "image.name" -}}
image: {{ .name }}
imagePullPolicy: {{ .pullPolicy }}
{{- end -}}