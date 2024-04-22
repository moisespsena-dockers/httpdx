FROM golang:1.22-bullseye

LABEL authors="moisespsena"

ARG PORT=80

COPY ./ /app

RUN set -eux; \
    cd /app && \
    go build -ldflags="-X main.buildTime=$(date +%s)" -o /bin/httpdx . && \
    rm -rf /app && \
    rm -rf /usr/local/go && \
    apt purge -y \
        g++ \
        gcc \
        libc6-dev \
        make \
        pkg-config

RUN mkdir /config && \
    httpdx create-config -server-addr $PORT -out "/config/httpdx.yml"

VOLUME /config

EXPOSE $PORT

WORKDIR /config

CMD ["httpdx"]