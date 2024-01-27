package config

type CacheConfig struct {
	LocalPort        int
	CacheDir         string
	MaxCacheSize     string
	InactiveDuration string

	LogLevel string

	Frontends     []Frontend
	Upstreams     []Upstream
	RepoProxies   []RepoProxy
	RepoProxyMaps map[string]bool
}

type Frontend struct {
	Listen          string
	CertificatePath string
	KeyPath         string
}

type Upstream struct {
	Name  string
	Hosts []string

	Repo bool
}

type RepoProxy struct {
	RepoID   string
	URL      string
	Upstream string

	TLS               bool
	CACertificatePath string
	CertificatePath   string
	KeyPath           string
	AuthHeader        string
}
