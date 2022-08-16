# whhand
Microservice GO runs as docker container (can work standalone as well) waits for github webhook event (with secret) then runs ansible playbook with some actions to deploy repo, make backups, restart service. To work properly with github, service has to have access to Internet IP address, so if it's IP inside, a firwall policy should be able to redirect Github Webhook Post requests to that IP inside.

For diagnostics Get requests from any web browser or console (from outside) can check its IP network
availability:

$ curl http://45.144.x.x:8989/webhook
FROM: 120.1x.x.x:6844 TO: 45.144.x.x:8989/WEBHOOK

This means service is available, github webhook event will be accepted and if it's ok 
necessary actions (ie ansible playbook, or whatever you need of such type) will 
be executed.

When run as docker container, playbook on host system can be run via fifo pipe 
which is created on mapped folder, in this case docker container can be run as:

Host folder eg /home/user/ansible which maps into container as eg /export 

Pipe is created (on Host) by: 
#### mkfifo my_exe_pipe
#### docker run -p 8989:8989 -v /home/user/ansible:/export -d whhand:arm64

Script exepipe.sh (in folder script_exepipe) has to be run on host 
to execute ansible playbook (or whatever)