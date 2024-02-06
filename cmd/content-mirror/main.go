package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift/content-mirror/pkg/config"
	"github.com/openshift/content-mirror/pkg/process"
	"github.com/openshift/content-mirror/pkg/watcher"
)

func main() {
	opt := &Options{
		Paths:        []string{"."},
		CacheDir:     "/tmp/cache",
		MaxCacheSize: "1g",
		CacheTimeout: "15m",
		Listen:       "8080",

		LocalPort: 9001,
	}
	cmd := &cobra.Command{
		Short: "Proxy RPM repositories and other important content",

		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opt.Paths = args
			}
			return opt.Run()
		},
	}

	cmd.Flags().StringVar(&opt.ConfigPath, "path", opt.ConfigPath, "The path to write the configuration to.")
	cmd.Flags().StringVar(&opt.CacheDir, "cache-dir", opt.CacheDir, "The directory to cache mirrored content into.")
	cmd.Flags().StringVar(&opt.MaxCacheSize, "max-size", opt.MaxCacheSize, "The maximum size of the cache (e.g. 10g, 100m).")
	cmd.Flags().StringVar(&opt.CacheTimeout, "timeout", opt.CacheTimeout, "How long an item is kept in the cache.")
	cmd.Flags().StringVar(&opt.Listen, "listen", opt.Listen, "The address (host:port, host, or port) to bind to for serving content.")
	cmd.Flags().BoolVarP(&opt.Verbose, "verbose", "v", opt.Verbose, "Display verbose output from the local server and nginx.")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type Options struct {
	Paths      []string
	ConfigPath string

	CacheDir     string
	MaxCacheSize string
	CacheTimeout string

	Listen    string
	LocalPort int
	Verbose   bool
}

// Run launches the configuration generator, the nginx process, and
// an HTTP server for dynamic content.
func (opt *Options) Run() error {
	t, err := template.New("config").Parse(nginxConfigTemplate)
	if err != nil {
		return err
	}

	level := "warn"
	if opt.Verbose {
		level = "debug"
	}
	cacheConfig := &config.CacheConfig{
		LogLevel:         level,
		LocalPort:        opt.LocalPort,
		CacheDir:         opt.CacheDir,
		MaxCacheSize:     opt.MaxCacheSize,
		InactiveDuration: opt.CacheTimeout,
		RepoProxyMaps:    make(map[string]bool),
		Frontends: []config.Frontend{
			{
				Listen: opt.Listen,
			},
		},
	}

	processor := process.New(opt.ConfigPath)
	generator := config.NewGenerator(opt.ConfigPath, t, cacheConfig)
	r := NewReloadManager(generator, processor)

	// Keep track of URLs that can be reached, in RepoProxyMaps
	go func() {
		for {
			lastConfig := generator.LastConfig()

			if lastConfig != nil {
				for _, repoProxy := range lastConfig.RepoProxies {
					url := repoProxy.URL

					client := &http.Client{}

					if len(repoProxy.CertificatePath) > 0 && len(repoProxy.KeyPath) > 0 {
						client = HttpTLSClient(repoProxy)
					}

					req, err := http.NewRequest("GET", url, nil)
					if err != nil {
						log.Println("Unable to make GET request", err)
						continue
					}

					if len(repoProxy.AuthHeader) > 0 {
						req.Header.Add("Authorization", repoProxy.AuthHeader)
					}

					response, responseErr := client.Do(req)
					if responseErr == nil && response.StatusCode == http.StatusOK {
						// We can reach the endpoint
						lastConfig.RepoProxyMaps[url] = true
					} else {
						_, exists := lastConfig.RepoProxyMaps[url]

						if exists {
							// If the url exists in the map, it means that we had connected that endpoint before
							// Since now we can't reach it, it means that the IP address changed, lets restart nginx
							command := "pkill"
							arg := "nginx"

							cmd := exec.Command(command, arg)
							_, err := cmd.Output()
							if err != nil {
								log.Printf("Error restarting nginx")
							}
							break
						}
					}
				}
			}

			// Repeat this process every 5 minutes
			time.Sleep(5 * time.Minute)
		}
	}()

	// the watcher coalesces frequent file changes
	w := watcher.New(opt.Paths, r.Load)
	w.SetMinimumInterval(10 * time.Millisecond)
	w.SetMaxDelays(100)

	if opt.LocalPort > 0 {
		handlers, err := NewHandlers(generator)
		if err != nil {
			return err
		}
		go func() {
			if err := http.ListenAndServe(fmt.Sprintf("localhost:%d", opt.LocalPort), handlers); err != nil && err != http.ErrServerClosed {
				log.Printf("error: server exited: %v", err)
				os.Exit(1)
			}
		}()
	}

	// only launch the process if we are generating a config file
	if len(opt.ConfigPath) > 0 {
		processor.Run()
	}

	return w.Run()
}

// Loader reads and generates a configuration for the given paths.
type Loader interface {
	Load(paths []string) error
}

// Reloader requests a reload.
type Reloader interface {
	Reload()
}

// reloadManager ties a Loader and Reloader together.
type reloadManager struct {
	loader   Loader
	reloader Reloader
}

// NewReloadManager ensures that the provided reloader is called whenever
// the configuration is loaded successfully.
func NewReloadManager(loader Loader, reloader Reloader) Loader {
	return &reloadManager{
		loader:   loader,
		reloader: reloader,
	}
}

func (m *reloadManager) Load(paths []string) error {
	if err := m.loader.Load(paths); err != nil {
		return err
	}
	m.reloader.Reload()
	return nil
}

func HttpTLSClient(repo config.RepoProxy) (client *http.Client) {
	x509cert, err := tls.LoadX509KeyPair(repo.CertificatePath, repo.KeyPath)
	if err != nil {
		panic(err.Error())
	}
	certs := []tls.Certificate{x509cert}
	if len(certs) == 0 {
		client = &http.Client{}
		return
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{Certificates: certs,
			InsecureSkipVerify: true},
	}
	client = &http.Client{Transport: tr}
	return
}
