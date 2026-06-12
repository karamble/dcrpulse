FROM getumbrel/tor:0.4.7.8@sha256:2ace83f22501f58857fa9b403009f595137fa2e7986c4fda79d82a8119072b6a

USER root

COPY torrc /etc/tor/torrc

RUN mkdir -p /data && chown -R 1000:1000 /data
