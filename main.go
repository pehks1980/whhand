/*
HookHandler - listen for github webhooks, run ansible-playbooky deployment script
it works when master branch push event !!!
raspberry pi cross build:

	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	   go build -o whhand_arm main.go;\

scp whhand_arm user@192.168.1.204:/home/user/ansible

check from remote side diagnostics:
curl http://x.x.x.x:8989/webhook
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"

	"github.com/google/go-github/github"
)

const ShellToUse = "/bin/sh" //"bash"

// config to handle jobs
type Job struct {
	WebhookPath string `yaml:"webhook_path"`
	Secret      string `yaml:"secret"`
	Command     string `yaml:"command"`
}

// config to handle jobs
type Config struct {
	Port string `yaml:"port"`
	Jobs []Job  `yaml:"jobs"`
}

// App - application contents & methods
type App struct {
	CTX      context.Context
	Config   *Config
	DeployMu sync.Mutex
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if err := validateConfig(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func validateConfig(config *Config) error {
	if strings.TrimSpace(config.Port) == "" {
		return fmt.Errorf("port is required")
	}
	if len(config.Jobs) == 0 {
		return fmt.Errorf("at least one job is required")
	}

	paths := make(map[string]struct{}, len(config.Jobs))
	for i, job := range config.Jobs {
		if strings.TrimSpace(job.WebhookPath) == "" {
			return fmt.Errorf("jobs[%d].webhook_path is required", i)
		}
		if !strings.HasPrefix(job.WebhookPath, "/") {
			return fmt.Errorf("jobs[%d].webhook_path must start with /", i)
		}
		if strings.TrimSpace(job.Secret) == "" {
			return fmt.Errorf("jobs[%d].secret is required", i)
		}
		if strings.TrimSpace(job.Command) == "" {
			return fmt.Errorf("jobs[%d].command is required", i)
		}
		if _, ok := paths[job.WebhookPath]; ok {
			return fmt.Errorf("duplicate webhook_path %q", job.WebhookPath)
		}
		paths[job.WebhookPath] = struct{}{}
	}

	return nil
}

func Shellout(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(ShellToUse, "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func main() {

	configFileDef := flag.String("config", "config.yml", "Path to configuration file")
	shutdownTimeout := flag.Int64("shutdown_timeout", 3, "shutdown timeout")
	flag.Parse()

	configFile := os.Getenv("CONFIG")
	if configFile == "" {
		log.Printf("Env CONFIG is not set. Using default: %s", *configFileDef)
		configFile = *configFileDef
	} else {
		log.Printf("Using env CONFIG = %s", configFile)
	}

	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	app := App{
		CTX:    context.Background(),
		Config: config,
	}

	log.Print("Starting the app for github webhooks..")

	serv := http.Server{
		Addr:              net.JoinHostPort("", app.Config.Port),
		Handler:           app.RegisterRoutesHTTP(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	// запуск сервера
	go func() {
		if err := serv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(interrupt)

	log.Printf("Started app at :%s", app.Config.Port)

	// ждет сигнала
	select {
	case err := <-serverErr:
		log.Fatalf("listen and serve err: %v", err)
	case sig := <-interrupt:
		log.Printf("Sig: %v, stopping app", sig)
	}

	// шат даун по контексту с тайм аутом
	ctx, cancel := context.WithTimeout(app.CTX, time.Duration(*shutdownTimeout)*time.Second)
	defer cancel()
	if err := serv.Shutdown(ctx); err != nil {
		log.Printf("shutdown err: %v", err)
	}
}

// RegisterRoutesHTTP - регистрация роутинга путей jobs для обработки сервером
func (app *App) RegisterRoutesHTTP() *mux.Router {
	r := mux.NewRouter()

	for _, job := range app.Config.Jobs {

		localJob := job

		// webhook
		r.HandleFunc(localJob.WebhookPath, app.hookHandlerPost(localJob)).Methods(http.MethodPost)
		// diag
		r.HandleFunc(localJob.WebhookPath, app.hookHandlerGet()).Methods(http.MethodGet)

	}

	return r
}

// HookHandler parses GitHub webhooks and sends an update to DeploymentMonitor.
// per each job from config yml
func (app *App) hookHandlerPost(job Job) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		/*
			payload, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("error reading request body: err=%s\n", err)
				return
			}
		*/
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()

		payload, err := github.ValidatePayload(r, []byte(job.Secret))
		if err != nil {
			log.Printf("Invalid payload: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		event, err := github.ParseWebHook(github.WebHookType(r), payload)
		if err != nil {
			log.Printf("Webhook parse error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch e := event.(type) {
		case *github.StatusEvent:
			var commitMessage string
			if e.Commit != nil {
				if e.Commit.Commit != nil {
					commitMessage = stringValue(e.Commit.Commit.Message)
				}
			}
			log.Printf("CommitUpdate status: %s, sha: %s, message: %s", stringValue(e.State), stringValue(e.SHA), commitMessage)
			return
		case *github.PullRequestEvent:
			// action when pull
			return
		case *github.PushEvent:
			ref := stringValue(e.Ref)
			if !strings.HasPrefix(ref, "refs/heads/") {
				log.Printf("push event has unsupported ref: %s", ref)
				w.WriteHeader(http.StatusAccepted)
				return
			}
			branch := ref[len("refs/heads/"):]
			if branch == "master" {
				sha := stringValue(e.After)
				go func() {
					app.DeployMu.Lock()
					defer app.DeployMu.Unlock()

					log.Printf("master branch push event, sha: %s", sha)
					out, errout, err := Shellout(job.Command)
					if err != nil {
						log.Printf("error: %v\n", err)
					}
					log.Printf("--- stdout ---: %s", out)
					log.Printf("--- stderr ---: %s", errout)
				}()
			}
			w.WriteHeader(http.StatusAccepted)
			return
		default:
			log.Printf("unknown WebHookType: %s, webhook-id: %s skipping", github.WebHookType(r), r.Header.Get("X-GitHub-Delivery"))
			return
		}
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// check app is responding
func (app *App) hookHandlerGet() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		line := "from: " + r.RemoteAddr + " to: " + r.Host + r.URL.String() + " ok! "
		log.Printf("diag test is OK - received message: %s\n", line)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": strings.ToUpper(line)})

	}
}
