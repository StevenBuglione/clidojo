FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      bash coreutils findutils grep sed gawk \
      less vim procps \
      ca-certificates \
      tar gzip \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 -s /bin/bash player

COPY dojo/bashrc /dojo/bashrc

ENV LANG=C.UTF-8
ENV LC_ALL=C
ENV TERM=xterm-256color

USER player
WORKDIR /work

CMD ["sleep", "infinity"]
