FROM scratch
COPY ca-certificates.crt /etc/ssl/certs/
COPY settings /settings
COPY main /
CMD ["telegraf", "--config", "https://us-west-2-1.aws.cloud2.influxdata.com/api/v2/telegrafs/04bca05d63e74000", "&&", "./main", "live"]