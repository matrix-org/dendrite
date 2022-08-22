{{- define "dendrite.names.key" -}}
    {{- default (printf "%s-key" (include "common.names.fullname" .)) .Values.dendrite.matrix_key_secret.existingSecret -}}
{{- end -}}
