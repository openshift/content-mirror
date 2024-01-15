package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"k8s.io/klog"
	"k8s.io/test-infra/prow/interrupts"
	"log"
	"net/http"
	"os"
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
	// prow registers this on init
	interrupts.OnInterrupt(func() { os.Exit(0) })

	if err := setupKubeconfigWatches(opt); err != nil {
		klog.Warningf("Failed to set up repo watches: %v", err)
	}

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
		Frontends: []config.Frontend{
			{
				Listen: opt.Listen,
			},
		},
	}

	processor := process.New(opt.ConfigPath)
	generator := config.NewGenerator(opt.ConfigPath, t, cacheConfig)
	r := NewReloadManager(generator, processor)

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

func setupKubeconfigWatches(opt *Options) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to set up watcher: %w", err)
	}
	for _, candidate := range opt.Paths {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if err := watcher.Add(candidate); err != nil {
			return fmt.Errorf("failed to watch %s: %w", candidate, err)
		}
	}

	go func() {
		for e := range watcher.Events {
			if e.Op == fsnotify.Chmod {
				// For some reason we get frequent chmod events from Openshift
				continue
			}
			klog.Infof("event: %s, Underlying repo changed. Exiting the pod, so that a new one can pick up the changes.", e.String())
			interrupts.Terminate()
		}
	}()

	return nil
}
