FROM scratch
COPY ca-certificates.crt /etc/ssl/certs/
COPY config.json /
COPY main /
CMD ["./main", "live"]