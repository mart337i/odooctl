ARG BASE=debian:bookworm-slim
FROM ${BASE}
RUN apt-get update && apt-get install -y --no-install-recommends squid ca-certificates && rm -rf /var/lib/apt/lists/*
COPY squid.conf /etc/squid/squid.conf
RUN chmod 0444 /etc/squid/squid.conf
ENTRYPOINT ["squid", "-N", "-f", "/etc/squid/squid.conf"]
