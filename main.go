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
	"flag"
	_ "fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
	CTX    context.Context
	Config *Config
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
	return &config, nil
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

	configFile := flag.String("config", "config.yml", "Path to configuration file")
	flag.Parse()

	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	app := App{
		CTX:    context.Background(),
		Config: config,
	}

	shutdownTimeout := flag.Int64("shutdown_timeout", 3, "shutdown timeout")

	log.Print("Starting the app for github webhooks..")

	serv := http.Server{
		Addr:    net.JoinHostPort("", app.Config.Port),
		Handler: app.RegisterRoutesHTTP(),
	}
	// запуск сервера
	go func() {
		if err := serv.ListenAndServe(); err != nil {
			log.Fatalf("listen and serve err: %v", err)
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	log.Printf("Started app at :%s", app.Config.Port)
	// ждет сигнала
	sig := <-interrupt

	log.Printf("Sig: %v, stopping app", sig)
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
		payload, err := github.ValidatePayload(r, []byte(job.Secret))
		if err != nil {
			log.Printf("Invalid payload: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()

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
					commitMessage = *e.Commit.Commit.Message
				}
			}
			log.Printf("CommitUpdate status: %s, sha: %s, message: %s", *e.State, *e.SHA, commitMessage)
			return
		case *github.PullRequestEvent:
			// action when pull
			return
		case *github.PushEvent:
			ref := *e.Ref
			branch := ref[len("refs/heads/"):]
			if branch == "master" {
				go func() {
					log.Printf("master branch push event, sha: %s", *e.After)
					out, errout, err := Shellout(job.Command)
					if err != nil {
						log.Printf("error: %v\n", err)
					}
					log.Printf("--- stdout ---: %s", out)
					log.Printf("--- stderr ---: %s", errout)
				}()
			}
			return
		default:
			log.Printf("unknown WebHookType: %s, webhook-id: %s skipping", github.WebHookType(r), r.Header.Get("X-GitHub-Delivery"))
			return
		}
	}
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
