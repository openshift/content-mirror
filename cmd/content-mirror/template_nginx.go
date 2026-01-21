package main

const nginxConfigTemplate = `
{{ $config := . -}}
worker_processes  5;  ## Default: 1
worker_rlimit_nofile 8192;
error_log stderr {{ .LogLevel }};
daemon off;

events {
  worker_connections  4096;  ## Default: 1024
}

http {
  sendfile     on;
  tcp_nopush   on;
  server_names_hash_bucket_size 128; # this seems to be required for some vhosts

  proxy_cache_path {{ .CacheDir }} levels=1:2 keys_zone=shared_cache:10m max_size={{ .MaxCacheSize }} inactive={{ .InactiveDuration }} use_temp_path=off;

  proxy_cache_use_stale error timeout http_500 http_502 http_503 http_504;
  proxy_cache_revalidate on;
  proxy_cache_min_uses 1;
  proxy_cache_background_update on;

{{- if gt .LocalPort 0 }}
  upstream localhost {
    keepalive 2;
    server localhost:{{ .LocalPort }};
  }
{{- end }}
{{ $upstreams := .Upstreams }}
{{ $repoProxies := .RepoProxies }}
{{- range .Upstreams }}
  upstream {{ .Name }} {
    keepalive 10;
    {{- range .Hosts }}
    server {{ . }};
    {{- end }}
  }
{{- end }}
{{- range .Frontends }}
  server {
    listen {{ .Listen }};

    {{- if gt (len .CertificatePath) 0 }}
    ssl_certificate {{ .CertificatePath }}
    ssl_certificate_key {{ .KeyPath }}
    {{- end }}

    proxy_cache shared_cache;

    # Allow keepalive
    proxy_http_version 1.1;
    # Remove the Connection header if the client sends it,
    # it could be "close" to close a keepalive connection
    proxy_set_header Connection "";

    {{ range $repoProxies -}}
    location ~ ^/{{ .RepoID }}(.+)$ {
      proxy_pass {{ .URL }}$1;

      proxy_ssl_server_name on;

      # Enable caching and report the status as a header
      proxy_cache_valid 200 302 {{ $config.InactiveDuration }};
      add_header X-Proxy-CacheConfig   $upstream_cache_status;
      proxy_set_header Host {{ .Upstream }};

      {{- if .AuthHeader }}
      proxy_set_header Authorization "{{ .AuthHeader }}";
      {{- end }}

      {{- if .TLS }}
      proxy_ssl_session_reuse on;
      {{- if gt (len .CACertificatePath) 0 }}
      proxy_ssl_verify       on;
      proxy_ssl_trusted_certificate {{ .CACertificatePath }};
      {{- end }}
      {{- if gt (len .CertificatePath) 0 }}
      proxy_ssl_certificate     {{ .CertificatePath }};
      proxy_ssl_certificate_key {{ .KeyPath }};
      {{- end }}
      {{- end }}

      # Do not cache repomd.xml for long. These need to be pulled from the
      # mirrored server regularly. When a yum repository is rebuilt, references in an old
      # copy of repomd.xml will no longer resolve - resulting in 404s.
      location ~ ^.*/(repodata/repomd\.xml) {
        proxy_pass {{ .URL }}$1;
        
        proxy_cache_valid 200 206 60s; 
        
        proxy_set_header Host {{ .Upstream }};
        {{- if .AuthHeader }}
        proxy_set_header Authorization "{{ .AuthHeader }}";
	    {{- end }}
        
        {{- if .TLS }}
        proxy_ssl_session_reuse on;
        {{- if gt (len .CACertificatePath) 0 }}
        proxy_ssl_verify       on;
        proxy_ssl_trusted_certificate {{ .CACertificatePath }};
        {{- end }}
        {{- if gt (len .CertificatePath) 0 }}
        proxy_ssl_certificate     {{ .CertificatePath }};
        proxy_ssl_certificate_key {{ .KeyPath }};
        {{- end }}
        {{- end }}
      
      }

    }
    location = /{{ .RepoID }} {
      rewrite ^ /{{ .RepoID }}/ redirect;
    }
    {{- if gt $config.LocalPort 0 }}
    location /{{ .RepoID }} {
      proxy_pass http://localhost;
      proxy_set_header Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{- end }}
    {{- end }}

    {{- if gt $config.LocalPort 0 }}
    location /healthz {
      proxy_pass http://localhost;
    }
    location = / {
      proxy_pass http://localhost;
      proxy_set_header Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }
    {{- end }}

    location / {
      return 404;
    }
  }
{{- end }}
}
`
