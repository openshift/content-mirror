package config

import (
	b64 "encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"strings"

	"github.com/go-ini/ini"
)

type RPMRepositorySection struct {
	ID            string `ini:"id"`
	Name          string
	BaseURL       string `ini:"baseurl"`
	Enabled       int
	SSLVerify     bool   `ini:"sslverify"`
	SSLClientKey  string `ini:"sslclientkey"`
	SSLClientCert string `ini:"sslclientcert"`
}

func getUsernamePassword(section *ini.Section) (string, error) {
	// If a dnf section contains username_file/password_file, the content of those
	// files is read and returned as "<username>:<password>". The non-standard keys
	// are also removed from the ini Section. If their are no auth key files,
	// "" is returned.
	const usernameFileKey = "username_file"
	const passwordFileKey = "password_file"
	usernamePassword := ""
	if section.HasKey(usernameFileKey) || section.HasKey(passwordFileKey) {
		if !(section.HasKey(usernameFileKey) && section.HasKey(passwordFileKey)) {
			return "", fmt.Errorf("%s and %s must both be specified", usernameFileKey, passwordFileKey)
		}
		// Load username from file and remove nonstandard key
		usernameIn, err := ioutil.ReadFile(section.Key(usernameFileKey).Value())
		if err != nil {
			return "", fmt.Errorf("unable to read %s: %v", usernameFileKey, err)
		}
		section.DeleteKey(usernameFileKey)

		// Load password from file and remove nonstandard key
		passwordIn, err := ioutil.ReadFile(section.Key(passwordFileKey).Value())
		if err != nil {
			return "", fmt.Errorf("unable to read %s: %v", passwordFileKey, err)
		}
		section.DeleteKey(passwordFileKey)
		usernamePassword = fmt.Sprintf("%s:%s", strings.TrimSpace(string(usernameIn)), strings.TrimSpace(string(passwordIn)))
	}
	return usernamePassword, nil
}

func LoadRPMRepoUpstreams(iniFile string) ([]Upstream, []RepoProxy, error) {
	var upstreams []Upstream
	var repoProxies []RepoProxy
	cfg, err := ini.Load(iniFile)
	if err != nil {
		return nil, nil, err
	}
	for _, section := range cfg.Sections() {
		if !section.Haskey("baseurl") {
			continue
		}
		usernamePassword, err := getUsernamePassword(section)
		if err != nil {
			return nil, nil, fmt.Errorf("%s can't load section %s authentication: %v", iniFile, section.Name(), err)
		}
		repo := &RPMRepositorySection{
			ID:      section.Name(),
			Enabled: 1,
		}
		if err := section.MapTo(repo); err != nil {
			return nil, nil, fmt.Errorf("%s can't load section %s: %v", iniFile, section.Name(), err)
		}
		if repo.Enabled == 0 {
			continue
		}
		var urls []*url.URL
		for _, u := range strings.Split(repo.BaseURL, " ") {
			u = strings.TrimSpace(u)
			if len(u) == 0 {
				continue
			}
			url, err := url.Parse(u)
			if err != nil {
				return nil, nil, fmt.Errorf("repo %s has a base URL that is not a valid URL: %v", iniFile, u)
			}
			if !strings.HasSuffix(url.Path, "/") {
				url.Path += "/"
			}
			urls = append(urls, url)
		}
		if len(urls) == 0 {
			return nil, nil, fmt.Errorf("repo %s has no baseurls", iniFile)
		}
		var hosts []string
		proxyPassURL := urls[0]
		for _, url := range urls {
			if url.Path == proxyPassURL.Path {
				if url.Scheme == "https" {
					if _, _, err := net.SplitHostPort(url.Host); err != nil {
						hosts = append(hosts, net.JoinHostPort(url.Host, "443"))
					} else {
						hosts = append(hosts, url.Host)
					}
				} else {
					hosts = append(hosts, url.Host)
				}
			}
		}
		if len(hosts) != len(urls) {
			log.Printf("one or more baseurls were omitted because they don't have a consistent path: %s", proxyPassURL.Path)
		}

		// nginx uses the Upstream name as the hostname for SSL connections. To support public
		// cloud CDN which depends on that hostname to forward requests appropriately, ensure
		// that the Upstream name matches a hostname.
		proxyPassURL.Host, _, err = net.SplitHostPort(hosts[0])
		if err != nil {
			return nil, nil, fmt.Errorf("unable to isolate host: %v", err)
		}

		upstream := Upstream{
			Repo:   true,
			Name:   proxyPassURL.Host,
			Hosts:  hosts,
		}

		repoProxy := RepoProxy{
			RepoID:   repo.ID,
			URL:      proxyPassURL.String(),
			Upstream: hosts[0],
		}

		if len(repo.SSLClientCert) > 0 {
			repoProxy.TLS = true
			repoProxy.CertificatePath = makePathRelativeToFile(iniFile, repo.SSLClientCert)
			repoProxy.KeyPath = makePathRelativeToFile(iniFile, repo.SSLClientKey)
		}
		if len(usernamePassword) > 0 {
			repoProxy.AuthHeader = "Basic " + b64.StdEncoding.EncodeToString([]byte(usernamePassword))
		}
		repoProxies = append(repoProxies, repoProxy)
		upstreams = append(upstreams, upstream)
	}

	return upstreams, repoProxies, nil
}
