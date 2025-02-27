FROM golang:1.23 AS modules

ADD go.mod go.sum /m/
RUN cd /m && go mod download
#cpy folder from modules to builder
FROM golang:1.23 AS builder
COPY --from=modules /go/pkg /go/pkg
#mkdir if its doesnt exist yet
RUN mkdir -p /whhand

ADD . /whhand

WORKDIR /whhand
#do things under unpriveleged 'user'
RUN useradd -u 1001 user
# Собираем бинарный файл GOARCH=arm64 pi | amd64 x86
# main - это результирующий exe файл в корне билдера
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /main
#chown exe to be owned by user
RUN chown user /main

#final dest layer image
FROM alpine
#copy user passwd info from builder
COPY --from=builder /etc/passwd /etc/passwd
# because program creates file inside we need to use image alpine which has commands to
# we make special dir with user rights
# otherwise exec cant create anything - perm denied as it creates process under this user
RUN mkdir -p /whhand
RUN chown user /whhand
USER user
WORKDIR /whhand

# exe file has to have its own name different from dir!
COPY --from=builder /main /whhand/main
# starting pathfile of the container
CMD ["/whhand/main"]

# docker build -t whhand .
# docker run -p 8989:8989 whhand
# docker ps
# exec -it c1a013d02149 /bin/bash
#
# build image for raspberry pi arm64: (cross to arm)
# docker buildx build --platform linux/arm64 -t whhand:arm64 .
# copy docker image  to raspberry system:
# save it in tar archive
# docker save whhand:arm64  > whhand-arm64.tar
# copy to pi via scp
# scp whhand-arm64.tar user@192.168.31.204:/home/user
# unpack tar to local image docker repo (on pi side)
# docker load -i whhand-arm64.tar
# check & run
# docker image ls | grep arm
# test with mount folder where pipe exists (created by: mkfifo my_exe_pipe)
# docker run -p 8989:8989 -v /home/user/ansible:/export -d whhand:arm64
# script has to be run at reboot time or manually - &
# @reboot /home/user/ansible/exepipe.sh
# save image to dockerhub
# docker login -u pehks1980 (pswd)
# docker tag  whhand:arm64 pehks1980/repo2:whhand_1
# docker push pehks1980/repo2:whhand_1
