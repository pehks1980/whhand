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
	"flag"
	_ "fmt"
	"github.com/gorilla/mux"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-github/github"
)

// App - application contents & methods
type App struct {
	CTX    context.Context
	cmdstr string
	secret string
}

const ShellToUse = "/bin/sh" //"bash"
// webhook secret
const secret1 = "my-secret-key-1980!A"
// port to listen
const whport = "8989"

// mode standalone app cmd to launch ansible
const shellcmd2 = "ansible-playbook -i '192.168.31.204, ' /home/user/ansible/play_depl.yml"

// docker container mode cmd to launch ansible should be sent to fifo pipe which should exist on mapped folder on host
// (inner is 'export' for this case)
const shellcmd1 = "echo \"ansible-playbook -i '192.168.31.204, ' /home/user/ansible/play_depl.yml\" > /export/my_exe_pipe"

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

	portDef := flag.String("webhook incoming port", whport, "WHPORT")

	port := os.Getenv("WHPORT")
	if port == "" {
		log.Printf("$WHPORT is not set. Using default: %s", *portDef)
		port = *portDef
	} else {
		log.Printf("Using env $WHPORT = %s", port)
	}

	secretDef := flag.String("webhook incoming secret", secret1, "WHSECRET")

	secret := os.Getenv("WHSECRET")
	if secret == "" {
		log.Printf("$WHSECRET is not set. Using default: ******")
		secret = *secretDef
	} else {
		log.Printf("Using env $WHSECRET = %s", secret)
	}

	shutdownTimeout := flag.Int64("shutdown_timeout", 3, "shutdown timeout")
    
	log.Print("Starting the app for github webhook")

	shellcmdDef := flag.String("webhook command", shellcmd1, "It can be also set as WHSHELLCMD.")

    shellcmd := os.Getenv("WHSHELLCMD")
	if shellcmd == "" {
		log.Printf("$WHSHELLCMD is not set. Using default (for Docker): %s", *shellcmdDef)
		shellcmd = *shellcmdDef
	} else {
		log.Printf("Using env $WHSHELLCMD = %s", shellcmd)
	}

	app := App{
		CTX:    context.Background(),
		cmdstr: shellcmd, //docker mode/standalone up to shell
		secret: secret,
	}

	log.Printf("cmd: %s", app.cmdstr)

	serv := http.Server{
		Addr:    net.JoinHostPort("", port),
		Handler: app.RegisterPublicHTTP(),
	}
	// запуск сервера
	go func() {
		if err := serv.ListenAndServe(); err != nil {
			log.Fatalf("listen and serve err: %v", err)
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	log.Printf("Started app at :%s", port)
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

// RegisterPublicHTTP - регистрация роутинга путей типа urls.py для обработки сервером
func (app *App) RegisterPublicHTTP() *mux.Router {
	r := mux.NewRouter()
	// webhook
	r.HandleFunc("/webhook", app.hookHandlerPost()).Methods(http.MethodPost)
	// diag
	r.HandleFunc("/webhook", app.hookHandlerGet()).Methods(http.MethodGet)
	return r
}

// HookHandler parses GitHub webhooks and sends an update to DeploymentMonitor.
func (app *App) hookHandlerPost() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		/*
			payload, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("error reading request body: err=%s\n", err)
				return
			}
		*/
		payload, err := github.ValidatePayload(r, []byte(app.secret))
		if err != nil {
			log.Printf("error validating request body: err=%s\n", err)
			return
		}
		defer r.Body.Close()

		event, err := github.ParseWebHook(github.WebHookType(r), payload)
		if err != nil {
			log.Printf("could not parse webhook: err=%s\n", err)
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
					out, errout, err := Shellout(app.cmdstr)
					if err != nil {
						log.Printf("error: %v\n", err)
					}
					log.Printf("--- stdout ---: %s", out)
					log.Printf("--- stderr ---: %s", errout)
				}()
			}
			return
		default:
			log.Printf("unknown WebHookType: %s, webhook-id: %s skipping\n", github.WebHookType(r), r.Header.Get("X-GitHub-Delivery"))
			return
		}
	}
}

// check app is responding
func (app *App) hookHandlerGet() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		line := "from: " + r.RemoteAddr + " to: " + r.Host + r.URL.String()
		log.Printf("diag test is OK - received message: %s\n", line)
		writeResponse(w, http.StatusOK, strings.ToUpper(line))
	}
}

func writeResponse(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(message))
	_, _ = w.Write([]byte("\n"))
}
