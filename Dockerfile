FROM debian:bullseye-slim
LABEL authors="moisespsena"

ARG PORT=80

COPY ./dist/httpdx_go1.22_bullseye /bin/httpdx

RUN set -eux; \
    apt update; \
    apt install -y curl \
        vim \
        nano \
        tmux \
        iproute2 \
        iputils-ping \
        tree; \
    rm -rf /var/lib/apt/lists/*;

RUN mkdir /config && \
    httpdx create-config -server-addr $PORT -out "/config/httpdx.yml"

VOLUME /config

EXPOSE $PORT

WORKDIR /config

CMD ["httpdx"]