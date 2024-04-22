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

WORKDIR /

RUN mkdir /config && \
    httpdx create-config -out "/config/httpdx.yml"

VOLUME /config

EXPOSE $PORT

CMD ["httpdx", "--config", "/config/httpdx.yml"]