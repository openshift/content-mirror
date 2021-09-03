package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

type Generator struct {
	configPath string
	template   *template.Template
	config     *CacheConfig

	lock       sync.Mutex
	lastConfig *CacheConfig
}

func NewGenerator(path string, template *template.Template, config *CacheConfig) *Generator {
	return &Generator{
		configPath: path,
		template:   template,
		config:     config,
	}
}

func (m *Generator) Load(paths []string) error {
	log.Printf("Configuration inputs changed")
	var upstreams []Upstream
	var repoProxies []RepoProxy
	upstreamDefined := make(map[string]bool)

	for _, p := range paths {
		files, err := ioutil.ReadDir(p)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			filePath := filepath.Join(p, file.Name())
			switch ext := path.Ext(file.Name()); ext {
			case ".repo":
				name := strings.TrimSuffix(file.Name(), ext)
				if len(name) == 0 {
					continue
				}
				rpmUpstreams, rpmRepoProxies, err := LoadRPMRepoUpstreams(filePath)
				if err != nil {
					return fmt.Errorf("%s: %v", filePath, err)
				}
				repoProxies = append(repoProxies, rpmRepoProxies...)
				for _, upstream := range rpmUpstreams {
					if _, ok := upstreamDefined[upstream.Name]; !ok {  // Dedupe upstreams
						upstreamDefined[upstream.Name] = true
						upstreams = append(upstreams, upstream)
					}
				}
			}
		}
	}

	config := *m.config
	config.Upstreams = upstreams
	config.RepoProxies = repoProxies
	buf := &bytes.Buffer{}
	if err := m.template.Execute(buf, config); err != nil {
		return err
	}
	if len(m.configPath) == 0 {
		log.Printf("template:\n%s", buf.String())
	} else {
		if err := ioutil.WriteFile(m.configPath, buf.Bytes(), 0640); err != nil {
			return err
		}
	}
	m.setLastConfig(&config)

	return nil
}

func (m *Generator) LastConfig() *CacheConfig {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.lastConfig
}

func (m *Generator) setLastConfig(config *CacheConfig) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.lastConfig = config
}

// makePathRelativeToFile makes a path reference out of a given file relative to the current working dir.
func makePathRelativeToFile(baseFile, path string) string {
	if len(path) == 0 {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	p := filepath.Join(filepath.Dir(baseFile), path)
	p, err := filepath.Abs(p)
	if err != nil {
		panic("unable to make absolute")
	}
	return p
}
