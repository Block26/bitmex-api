FROM scratch
COPY ca-certificates.crt /etc/ssl/certs/
COPY settings /settings
COPY main /
CMD ["./main", "live"]