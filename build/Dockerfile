FROM alpine

RUN apk update && apk add tzdata && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone

ADD promtail /opt/promtail

ENTRYPOINT ["/opt/promtail"]
