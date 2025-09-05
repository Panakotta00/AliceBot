FROM scratch
COPY AliceBot /AliceBot
WORKDIR /data
ENTRYPOINT ["/AliceBot"]