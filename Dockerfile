FROM golang:alpine as base_stage

RUN apk update && apk add tzdata && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone && mkdir /data

COPY . /data

ENV CGO_ENABLED=0

RUN cd /data/clients/cmd/promtail && go build -v . 

# FROM alpine

# COPY --from=base_stage /data/clients/cmd/promtail/promtail  /opt/promtail


