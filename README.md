# whhand (webhook handler go)
Microservice GO runs as docker container (can work standalone as well) waits for github webhook event (with secret) then runs ansible playbook with some actions to deploy repo, 
make backups, restart service. To work properly with github, service has to have access to Internet IP address, so if it's IP inside, 
a firewall policy (TCP PORT REDIRECT) should be able to redirect Github Webhook http post requests to that IP inside.

If iptables is used as firewall, the following Rule that redirects requests 
from Host with Internet IP to IP inside (this case port 8989, IP inside 10.x.x.x):

sudo iptables -A PREROUTING -p tcp -m tcp --dport 8989 -j DNAT --to-destination 10.x.x.x:8989

For diagnostics app can accept a Get request from any web browser or console (from outside IP) so it can check its IP network
availability:

For eg IP 45.144.x.x is Internet Address of your side for github webhooks:

$ curl http://45.144.x.x:8989/webhook

If its ok, you'll get response:

FROM: 120.1x.x.x:6844 TO: 45.144.x.x:8989/WEBHOOK

This means service is available and ready to accept requests from github webhook 
events.

Necessary actions (ie ansible playbook, or whatever you need of such type) will 
be executed (if you set fifo pipe on your host, and execute script exepipe.sh).

When run as docker container, playbook on host system can be run via fifo pipe 
which is created on a mapped folder, in this case docker container can be run as:

#### docker run -p 8989:8989 -v /home/user/ansible:/export -d whhand:arm64

Host folder eg /home/user/ansible maps into container as eg /export 

Pipe is created (in the host folder) by: 
#### mkfifo my_exe_pipe

Script exepipe.sh (located in folder script_exepipe) has to be run on host 
to execute commands from docker conansible playbook (or whatever). 
It waits for command from docker app container on this pipe and executes it.

