# whhand

`whhand` is a small Go webhook handler for GitHub push events. It validates the
GitHub webhook signature, checks that the push is for the `master` branch, and
then runs a configured deployment command.

The project is designed for small servers such as Raspberry Pi devices, where a
full CI/CD system may be more than you need. It can run directly on the host or
inside Docker.

## How It Works

1. GitHub sends a webhook request to one of the configured webhook paths.
2. `whhand` validates the request using the job secret from the YAML config.
3. If the event is a push to `master`, the job command is executed.
4. Deployment commands are serialized so that two pushes do not run overlapping
   deployments.

When running in Docker, the container can write a command to a FIFO pipe mounted
from the host. The host runs `script_exepipe/exepipe.sh`, reads the command from
that pipe, validates it, and executes it locally.

## Configuration

The application uses a YAML config file. By default it reads `config.yml` from
the current working directory. You can override this with the `CONFIG`
environment variable.

Docker images default to:

```sh
CONFIG=/export/config.yml
```

Example:

```yaml
port: "8989"

jobs:
  - webhook_path: "/webhook"
    secret: "my-github-webhook-secret"
    command: "echo \"ansible-playbook -i '192.168.31.204,' /home/user/ansible/play_depl.yml\" > /export/my_exe_pipe"

  - webhook_path: "/webhook-other"
    secret: "my-other-github-webhook-secret"
    command: "echo \"ansible-playbook -i '192.168.31.204,' /home/user/ansible/play_depl_other.yml\" > /export/my_exe_pipe"
```

Config fields:

- `port`: TCP port for the HTTP server.
- `jobs[].webhook_path`: URL path for this webhook, for example `/webhook`.
- `jobs[].secret`: GitHub webhook secret for request validation.
- `jobs[].command`: command executed after a valid `master` branch push.

## GitHub Webhook Setup

In the GitHub repository settings:

1. Open `Settings` -> `Webhooks` -> `Add webhook`.
2. Set the payload URL, for example `http://your.server.ip:8989/webhook`.
3. Set `Content type` to `application/json`.
4. Set `Secret` to the same value used in `config.yml`.
5. Select push events.

The service currently deploys only pushes to the `master` branch.

## Run Locally

```sh
go run . -config script_exepipe/config.yml
```

You can also use the environment variable:

```sh
CONFIG=script_exepipe/config.yml go run .
```

Optional shutdown timeout:

```sh
go run . -config script_exepipe/config.yml -shutdown_timeout 5
```

## Run With Docker

Build for the current platform:

```sh
docker build -t whhand .
```

Run with a host directory mounted as `/export`:

```sh
docker run -p 8989:8989 -v /home/user/ansible:/export -d whhand
```

The mounted host directory should contain:

- `config.yml`
- `my_exe_pipe`
- any playbooks or scripts used by the deployment command

Create the FIFO pipe on the host:

```sh
mkfifo /home/user/ansible/my_exe_pipe
```

For Raspberry Pi or another ARM64 target:

```sh
docker buildx build --platform linux/arm64 -t whhand:arm64 .
```

## Host-Side Command Runner

If the container writes commands into `/export/my_exe_pipe`, run the pipe reader
on the host:

```sh
script_exepipe/exepipe.sh
```

The script logs to:

```sh
/home/user/ansible/exepipe.log
```

By default, `exepipe.sh` allows `ansible-playbook` commands. You can override
the allowlist:

```sh
ALLOWED_CMDS="ansible-playbook /usr/bin/ansible-playbook" script_exepipe/exepipe.sh
```

The script rejects shell control characters such as `;`, `&`, `|`, redirects,
command substitution, and environment expansion before executing a command.

## Diagnostics

Each configured webhook path also accepts `GET` requests as a simple health
check:

```sh
curl http://your.server.ip:8989/webhook
```

Example response:

```json
{"status":"FROM: 127.0.0.1:56460 TO: 127.0.0.1:8989/WEBHOOK OK! "}
```

## Network Notes

GitHub must be able to reach the webhook endpoint. If the service runs on a
private network, configure port forwarding from a public address to the internal
host.

Example `iptables` redirect:

```sh
sudo iptables -t nat -A PREROUTING -p tcp --dport 8989 -j DNAT --to-destination 10.x.x.x:8989
```

Your exact firewall and NAT setup will depend on your network.

## Development

Run tests:

```sh
go test ./...
```

Run vet:

```sh
go vet ./...
```
