ARG BRCLIENTD_BASE=latest
FROM ghcr.io/karamble/brclientd:${BRCLIENTD_BASE}
COPY supervisor.sh /supervisor.sh
ENTRYPOINT ["/bin/sh", "/supervisor.sh"]
