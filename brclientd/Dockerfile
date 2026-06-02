ARG BRCLIENTD_BASE=0.1.0
FROM ghcr.io/karamble/brclientd:${BRCLIENTD_BASE}
COPY supervisor.sh /supervisor.sh
ENTRYPOINT ["/bin/sh", "/supervisor.sh"]
