FROM ghcr.io/getumbrel/tor:0.4.9.11@sha256:e382b8629c0dfef6ceb396b062622d4e4e955b19d6f16b883fd2c0723ad5671a

USER root

COPY torrc /etc/tor/torrc

RUN mkdir -p /data && chown -R 1000:1000 /data
